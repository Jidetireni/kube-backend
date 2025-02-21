package models

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

func GetKubernetesClient() (*kubernetes.Clientset, *rest.Config, error) {
	// Load kubeconfig file (replace with your kubeconfig path)
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, nil, err
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}

	return clientset, config, nil
}

func CreateNamespace(clientset *kubernetes.Clientset, projectname string, user_id int) (string, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%v-ns", projectname, user_id),
		},
	}

	_, err := clientset.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	return ns.Name, err
}

func CreateConfigMap(clientset *kubernetes.Clientset, namespace, repoUrl, projectName, lang string) (string, error) {
	script := fmt.Sprintf(
		`#!/bin/bash
	
		echo "Starting application setup..."
		apt update && apt install -y git
	
		# Pull the latest code
		git clone %s /app
		
		cd /app
		git pull %s
	
		# Run your application or tests
		if [ "%s" == "python" ]; then
			pip install -r requirements.txt
			python main.py
		elif [ "%s" == "node" ]; then
			npm install
			npm start
		fi`,
		repoUrl, repoUrl, lang, lang)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-cfm", projectName),
			Namespace: namespace,
		},
		Data: map[string]string{
			"setup.sh": script,
		},
	}

	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create config map: %v", err)
	}

	return configMap.Name, err
}

func DeployPodsAndServices(clientset *kubernetes.Clientset, namespace, projectName, env, lang, configMap string) (string, error) {
	// Pod name and label
	appName := fmt.Sprintf("%s-%s-app", env, projectName)

	// Deploy application pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":     appName,
				"env":     env,
				"purpose": "application",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  fmt.Sprintf("%s-container", appName),
					Image: fmt.Sprintf("%s:latest", lang), // Use the language from the payload
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 8080,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "script-volume",
							MountPath: "/scripts",
						},
					},
					Command: []string{"python", "-c", "import time; time.sleep(999999)"},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "script-volume",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: configMap,
							},
							DefaultMode: ptrToInt32(0777),
						},
					},
				},
			},
		},
	}

	_, err := clientset.CoreV1().Pods(namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create pod: %v", err)
	}

	// Create a Service to expose the pod
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", appName),
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": appName, // Matches the pod label
			},
			Ports: []corev1.ServicePort{
				{
					Port:       80,                   // External port
					TargetPort: intstr.FromInt(8080), // Pod's port
				},
			},
			Type: corev1.ServiceTypeClusterIP, // Internal service (Change to NodePort or LoadBalancer if needed)
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create service: %v", err)
	}

	return appName, nil
}

func ptrToInt32(i int32) *int32 {
	return &i
}

func ExecIntoContainer(clientset *kubernetes.Clientset, config *rest.Config, appName, namespace string) (string, string, error) {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(appName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: fmt.Sprintf("%s-container", appName),
			Command:   []string{"/bin/bash", "-c", "cd /scripts && chmod +x setup.sh && ./setup.sh"},
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, metav1.ParameterCodec)

	// Create a SPDY executor
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("error creating SPDY executor: %w", err)
	}

	// Set a context for better resource management
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Stdin:  os.Stdin,
		Tty:    false,
	})

	// Return both outputs and any error
	if err != nil {
		return stdout.String(), stderr.String(), fmt.Errorf("command execution failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), stderr.String(), nil

}
