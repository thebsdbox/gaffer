package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

type deploymentPlan struct {
	Label      string           `json:"label"`
	Version    string           `json:"version"`
	Deployment []DeploymentTask `json:"deployment"`
	VMWConfig  VMConfig         `json:"vmconfig,omitempty"`
}

// VMConfig - f
type VMConfig struct {
	VCenterURL     *string `json:"vcenterURL,omitempty"`
	DCName         *string `json:"datacentre,omitempty"`
	DSName         *string `json:"datastore,omitempty"`
	NetworkName    *string `json:"network,omitempty"`
	VSphereHost    *string `json:"host,omitempty"`
	Template       *string `json:"template,omitempty"`
	VMTemplateAuth struct {
		Username *string `json:"guestUser,omitempty"`
		Password *string `json:"guestPass,omitempty"`
	} `json:"guestCredentials,omitempty"`
}

// DeploymentTask - is passed to the vSphere API functions in order to be executed on a remote VM
type DeploymentTask struct {
	Name string `json:"name"`
	Note string `json:"note"`
	Task struct {
		InputTemplate string `json:"inputTemplate"`
		OutputName    string `json:"outputName"`
		OutputType    string `json:"outputType"`

		Version  string              `json:"version"`
		Commands []DeploymentCommand `json:"commands"`
	} `json:"task"`
}

// DeploymentCommand - is passed to the vSphere API functions in order to be executed on a remote VM
type DeploymentCommand struct {
	CMDType string `json:"type"` //defines the type of command
	CMDNote string `json:"note"` //defines a notice that the end user will recieve

	CMDPath string `json:"path"` //path to either an executable or file to download

	CMDDelete bool `json:"delAfterDownload"` //remove the file once downloaded

	CMDArgs  string `json:"args"`  //arguments to pass to the executable
	CMDWatch bool   `json:"watch"` //watch the pid to ensure it executes correctly
}

var plan *deploymentPlan
var deploymentCounter int //defaults to 0
var commandCounter int    //defaults to 0

// InitDeployment - Allocates barebones deployment plan
func InitDeployment() {
	plan = new(deploymentPlan)
}

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
	if plan == nil {
		log.Printf("Code isn't initialising the Deployment Plan, intitialising automatically")
		InitDeployment()
	}
	err = jsonParser.Decode(&plan)
	if err != nil {
		return fmt.Errorf("Error Parsing JSON: %v", err)
	}

	log.Printf("Finished parsing [%s], [%d] deployments will be ran", plan.Label, len(plan.Deployment))
	return nil
}

//NextDeployment - This will return the Command Path, the Arguments or an error
func NextDeployment() *DeploymentTask {
	if deploymentCounter > len(plan.Deployment) {
		return nil
	}

	defer func() { deploymentCounter++ }()
	return &plan.Deployment[deploymentCounter]
}

// DeploymentCount - Returns the number of commands to be executed for use in a loop
func DeploymentCount() int {
	return len(plan.Deployment)
}

//NextCommand - This will return the Command Path, the Arguments or an error
func NextCommand(deployment *DeploymentTask) *DeploymentCommand {
	if commandCounter > len(deployment.Task.Commands) {
		commandCounter = 0 // reset counter for next set of commands
		return nil
	}

	defer func() { commandCounter++ }()
	return &deployment.Task.Commands[commandCounter]
}

// CommandCount - Returns the number of commands to be executed for use in a loop
func CommandCount(deployment *DeploymentTask) int {
	return len(deployment.Task.Commands)
}

//VMwareConfig -
func VMwareConfig() *VMConfig {
	return &plan.VMWConfig
}
