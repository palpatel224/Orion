package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Task is a lightweight representation of the JSON returned by your Manager.
// Defining it here ensures your CLI is fully decoupled from the core engine.
type Task struct {
	ID          string `json:"ID"`
	Name        string `json:"Name"`
	State       string `json:"State"`
	Image       string `json:"Image"`
	ContainerId string `json:"ContainerId"`
}

// Client wraps the HTTP connection to the Manager
type Client struct {
	ManagerAddr string
}

// NewClient initializes a new CLI client
func NewClient(addr string) *Client {
	return &Client{ManagerAddr: addr}
}

// GetTasks fetches all tasks from the Manager's API
func (c *Client) GetTasks() ([]*Task, error) {
	url := fmt.Sprintf("%s/tasks", c.ManagerAddr)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to manager at %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manager returned status: %d", resp.StatusCode)
	}

	var tasks []*Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	return tasks, nil
}

// SubmitTask sends a raw JSON payload to the Manager to schedule a new task
func (c *Client) SubmitTask(taskJSON []byte) error {
	url := fmt.Sprintf("%s/tasks", c.ManagerAddr)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(taskJSON))
	if err != nil {
		return fmt.Errorf("failed to submit task: %w", err)
	}
	defer resp.Body.Close()

	// Accept either 200 OK or 201 Created as success
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("manager rejected task (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// SubmitApp sends a JSON payload to the Manager to create an app.
func (c *Client) SubmitApp(appJSON []byte) error {
	url := fmt.Sprintf("%s/app", c.ManagerAddr)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(appJSON))
	if err != nil {
		return fmt.Errorf("failed to submit app: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("manager rejected app (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
