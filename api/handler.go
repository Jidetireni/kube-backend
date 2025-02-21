package api

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"backend.im/models"
	"github.com/gin-gonic/gin"
)

type GitInfo struct {
	RepoURL  string `json:"repo_url"`
	CommitID string `json:"commit_id"`
}

type BackendRequest struct {
	UserID      int     `json:"user_id"`
	GitInfo     GitInfo `json:"git_info"`
	Language    string  `json:"language"`
	Database    string  `json:"database"`
	Environment string  `json:"environment"`
	ProjectName string  `json:"project_name"`
}

type RequestBody struct {
	Data BackendRequest `json:"data"`
}

var orcUrl string

func init() {
	orcUrl = os.Getenv("ORCHESTRATION_URL")
	if orcUrl == "" {
		log.Fatal("ORCHESTRATION_URL environment variable is not set")
	}

}

func Runk8s(c *gin.Context) {
	var reqBody RequestBody

	// Bind the JSON body to the struct
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Access the data from the request body
	data := reqBody.Data

	// Initialize Kubernetes client
	clientset, config, err := models.GetKubernetesClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize Kubernetes client", "msg": err.Error()})
		return
	}

	ns, err := models.CreateNamespace(clientset, data.ProjectName, data.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create namespace"})
		return
	}

	confMap, err := models.CreateConfigMap(clientset, ns, data.GitInfo.RepoURL, data.ProjectName, data.Language)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create configmap"})
		return
	}

	// Deploy pods and services
	appName, err := models.DeployPodsAndServices(clientset, ns, data.ProjectName, data.Environment, data.Language, confMap)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to deploy pods and services"})
		return
	}

	log_output, log_error, err := models.ExecIntoContainer(clientset, config, appName, ns)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to execute test", "msg": err.Error()})
		return
	}

	err = sendResponse(log_output, log_error, &data.GitInfo, orcUrl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": " send response back to the orchestration app"})
		return
	}

	// Respond with a success message
	c.JSON(http.StatusOK, gin.H{
		"message": "Backend created successfully",
		"data":    data,
	})
}

func Testk8s(c *gin.Context) {
	var reqBody RequestBody

	// Bind the JSON body to the struct
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Access the data from the request body
	data := reqBody.Data

	// Initialize Kubernetes client
	clientset, config, err := models.GetKubernetesClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize Kubernetes client"})
		return
	}

	appName := fmt.Sprintf("%s-%s-app", data.Environment, data.ProjectName)
	ns := fmt.Sprintf("%s-%v-ns", data.ProjectName, data.UserID)

	log_output, log_error, err := models.ExecIntoContainer(clientset, config, appName, ns)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to execute test"})
		return
	}

	err = sendResponse(log_output, log_error, &data.GitInfo, orcUrl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": " send response back to the orchestration app"})
		return
	}

	// Respond with a success message
	c.JSON(http.StatusOK, gin.H{
		"message": "Backend created successfully",
		"data":    data,
	})
}
