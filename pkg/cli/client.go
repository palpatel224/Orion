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

// Node represents a worker node in the cluster
type Node struct {
	ID        string `json:"ID"`
	Address   string `json:"Address"`
	Heartbeat string `json:"Heartbeat"`
}

// GetNodes fetches the list of active worker nodes from the manager
func (c *Client) GetNodes() ([]Node, error) {
	url := fmt.Sprintf("%s/nodes", c.ManagerAddr)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status: %d", resp.StatusCode)
	}

	var nodes []Node
	if err := json.NewDecoder(resp.Body).Decode(&nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

// StopTask sends a request to gracefully terminate a task
func (c *Client) StopTask(taskID string) error {
	url := fmt.Sprintf("%s/tasks/%s", c.ManagerAddr, taskID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to stop task, server returned status: %d", resp.StatusCode)
	}
	return nil
}
