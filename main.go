package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/guest"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

func main() {
	InitDeployment()
	vm := VMwareConfig() //Pull VMware configuration from JSON

	cmd := &cobra.Command{
		Use:   "dockerVM <flags> deployment.json",
		Short: "This will take an existing VMware template (RHEL/CentOS (today)), update and prepare it for Docker-CE",
		Run: func(cmd *cobra.Command, args []string) {
			// Use the only argument
			err := OpenFile(args[0])
			if err != nil {
				log.Fatalf("%v", err)
			}

			if *vm.VCenterURL == "" || *vm.DCName == "" || *vm.DSName == "" || *vm.VSphereHost == "" || len(args) != 1 {
				cmd.Usage()
				os.Exit(1)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			client, err := vCenterLogin(ctx, *vm)
			if err != nil {
				log.Fatalf("%v", err)
			}

			runTasks(ctx, client)
			//runCommands
		},
	}
	// Overlay from
	vm.VCenterURL = cmd.Flags().String("vcurl", os.Getenv("VCURL"), "VMware vCenter URL, format https://user:pass@address/sdk [REQD]")
	vm.DCName = cmd.Flags().String("datacenter", os.Getenv("VCDATACENTER"), "The name of the Datacenter to host the VM [REQD]")
	vm.DSName = cmd.Flags().String("datastore", os.Getenv("VCDATASTORE"), "The name of the DataStore to host the VM [REQD]")
	vm.NetworkName = cmd.Flags().String("network", os.Getenv("VCNETWORK"), "The network label the VM will use [REQD]")
	vm.VSphereHost = cmd.Flags().String("hostname", os.Getenv("VCHOST"), "The server that will run the VM [REQD]")
	vm.VMTemplateAuth.Username = cmd.Flags().String("templateUser", os.Getenv("VMUSER"), "A created user inside of the VM template")
	vm.VMTemplateAuth.Password = cmd.Flags().String("templatePass", os.Getenv("VMPASS"), "The password for the specified user inside the VM template")

	log.Println("Starting Docker VMware deployment")
	err := cmd.Execute()
	if err != nil {
		log.Fatalf("Error parsing the flags")
	}

}

func runTasks(ctx context.Context, client *govmomi.Client) {
	taskCount := DeploymentCount()
	vm := VMwareConfig() //Pull VMware configuration from JSON
	for i := 0; i < taskCount; i++ {
		task := NextDeployment()
		log.Printf("Building an updated Image with Docker-CE")
		newVM, err := provision(ctx, client, *vm, task.Task.InputTemplate, task.Task.OutputName)

		if err != nil {
			log.Printf("Provisioning has failed =>")
			log.Fatalf("%v", err)
		}
		if task.Task.OutputType == "Template" {
			err = newVM.ShutdownGuest(ctx)
			if err != nil {
				log.Printf("Provisioning has failed =>")
				log.Fatalf("%v", err)
			}
			err = newVM.MarkAsTemplate(ctx)
			if err != nil {
				log.Printf("Provisioning has failed =>")
				log.Fatalf("%v", err)
			}
		}
		auth := &types.NamePasswordAuthentication{
			Username: *vm.VMTemplateAuth.Username,
			Password: *vm.VMTemplateAuth.Password,
		}

		if task != nil {
			log.Printf("Beginning Task [%s]: %s", task.Name, task.Note)
			runCommands(ctx, client, newVM, auth, task)
		}
	}
}

func runCommands(ctx context.Context, client *govmomi.Client, vm *object.VirtualMachine, auth *types.NamePasswordAuthentication, deployment *DeploymentTask) {
	cmdCount := CommandCount(deployment)
	log.Printf("%d commands will be executed.", cmdCount)
	for i := 0; i < cmdCount; i++ {
		cmd := NextCommand(deployment)
		// if cmd == nil then no more commands to run
		if cmd != nil {
			if cmd.CMDNote != "" { // If the command has a note, then print it out
				log.Printf("Task: %s", cmd.CMDNote)
			}
			switch cmd.CMDType {
			case "execute":
				pid, err := vmExec(ctx, client, vm, auth, cmd.CMDPath, cmd.CMDArgs)
				if err != nil {
					log.Fatalf("%v", err)
				}
				if cmd.CMDWatch == true {
					watchPid(ctx, client, vm, auth, []int64{pid})
				}
			case "download":
				vmDownloadFile(ctx, client, vm, auth, cmd.CMDPath, cmd.CMDDelete)
			}
			// Execute the command on the Virtual Machine

		}
	}
}

func vmExec(ctx context.Context, client *govmomi.Client, vm *object.VirtualMachine, auth *types.NamePasswordAuthentication, path string, args string) (int64, error) {
	o := guest.NewOperationsManager(client.Client, vm.Reference())
	pm, _ := o.ProcessManager(ctx)

	cmdSpec := types.GuestProgramSpec{
		ProgramPath: path,
		Arguments:   args,
	}

	pid, err := pm.StartProgram(ctx, auth, &cmdSpec)
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// This will download a file from the Virtual Machine to the localhost
func vmDownloadFile(ctx context.Context, client *govmomi.Client, vm *object.VirtualMachine, auth *types.NamePasswordAuthentication, path string, deleteonDownload bool) error {
	o := guest.NewOperationsManager(client.Client, vm.Reference())
	fm, _ := o.FileManager(ctx)
	fileDetails, err := fm.InitiateFileTransferFromGuest(ctx, auth, path)
	if err != nil {
		return err
	}
	log.Printf("%d of file [%s] downloaded succesfully", fileDetails.Size, fileDetails.Url)
	log.Printf("Removing file [%s] from Virtual Machine", path)
	if deleteonDownload == true {
		err = fm.DeleteFile(ctx, auth, path)
		if err != nil {
			return err
		}
	}
	return nil
}

func watchPid(ctx context.Context, client *govmomi.Client, vm *object.VirtualMachine, auth *types.NamePasswordAuthentication, pid []int64) error {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o := guest.NewOperationsManager(client.Client, vm.Reference())
	pm, _ := o.ProcessManager(ctx)

	process, err := pm.ListProcesses(ctx, auth, pid)
	if err != nil {
		return err
	}
	if len(process) > 0 {
		log.Printf("Watching process [%d] cmd [%s]\n", process[0].Pid, process[0].CmdLine)
		fmt.Printf(".")
	} else {
		log.Fatalf("Process couldn't be found running")
	}

	// Counter if VMtools loses a previously watched process
	processTimeout := 0

	for {
		time.Sleep(5 * time.Second)
		process, err = pm.ListProcesses(ctx, auth, pid)

		if err != nil {
			return err
		}
		// Watch Process
		if process[0].EndTime == nil {
			fmt.Printf(".")
		} else {
			if process[0].ExitCode != 0 {
				fmt.Printf("\n")
				log.Println("Return code was not zero, please investigate logs on the Virtual Machine")
				break
			} else {
				fmt.Printf("\n")
				log.Println("Process completed Successfully")
				return nil
			}
		}
		// Process, now can't be found...
		if len(process) == 0 {
			fmt.Printf("x")
			processTimeout++
			if processTimeout == 12 { // 12x5 seconds == 60 second time out
				fmt.Printf("\n")
				log.Println("Process no longer watched, VMware Tools may have been restarted")
				break
			}
		}
	}
	return nil
}
