package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type responseMessage struct {
	Status  string  `json:"status"`
	Step    string  `json:"step"`
	Logs    string  `json:"details,omitempty"` // Optional field
	GitInfo GitInfo `json:"git_info"`
}

func sendResponse(log_output, log_error string, gitinfo *GitInfo, url string) error {

	var status, logs string
	if log_output == "" {
		status = "failed"
		logs = log_error
	} else {
		status = "success"
		logs = log_output
	}

	resp := responseMessage{
		Status:  status,
		Step:    "response from k8s",
		Logs:    logs,
		GitInfo: *gitinfo,
	}
	jsonResp, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonResp))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("orc returned unexpected status: %s", response.Status)
	}

	log.Printf("Request sent successfully to orch (%s)", url)
	return nil
}
