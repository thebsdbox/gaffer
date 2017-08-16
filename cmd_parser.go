package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

type deploymentPlan struct {
	Label        string              `json:"label"`
	Version      string              `json:"version"`
	CommandCount int                 `json:"commandcount,omitempty"`
	Commands     []DeploymentCommand `json:"commands"`
}

// DeploymentCommand - is passed to the vSphere API functions in order to be executed on a remote VM
type DeploymentCommand struct {
	CMDNote  string `json:"note"`
	CMDPath  string `json:"path"`
	CMDArgs  string `json:"args"`
	CMDWatch bool   `json:"watch"`
}

var plan *deploymentPlan
var commandCounter int

//OpenFile - This will open a file, check file can be read and also checks the format
func OpenFile(filePath string) error {

	// Attempt to open file
	deploymentFile, err := os.Open(filePath)
	defer deploymentFile.Close()
	if err != nil {
		return err
	}
	// Attempt to parse JSON
	jsonParser := json.NewDecoder(deploymentFile)
	plan = new(deploymentPlan)
	err = jsonParser.Decode(&plan)
	if err != nil {
		return fmt.Errorf("Error Parsing JSON: %v", err)
	}

	log.Printf("Finished parsing [%s], [%d] commands will be ran", plan.Label, len(plan.Commands))
	return nil
}

//NextCommand - This will return the Command Path, the Arguments or an error
func NextCommand() *DeploymentCommand {
	if commandCounter > len(plan.Commands) {
		return nil
	}

	defer func() { commandCounter++ }()
	return &plan.Commands[commandCounter]
}

// CommandCount - Returns the number of commands to be executed for use in a loop
func CommandCount() int {
	return len(plan.Commands)
}
