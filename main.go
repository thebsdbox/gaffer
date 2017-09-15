package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/vmware/govmomi/vim25/soap"

	"github.com/spf13/cobra"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/guest"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

var cmdResults = map[string]string{}

func main() {
	InitDeployment()
	vm := VMwareConfig() //Pull VMware configuration from JSON

	cmd := &cobra.Command{
		Use:   "dockerVM deployment.json",
		Short: "This will take an existing VMware template (RHEL/CentOS (today)), update and prepare it for Docker-CE",
		Run: func(cmd *cobra.Command, args []string) {
			// Check that the argument (the json file exists)
			if len(args) == 0 {
				cmd.Usage()
				log.Fatalf("Please specify the path to a configuration file")
			}
			err := OpenFile(args[0])
			if err != nil {
				log.Fatalf("%v", err)
			}

			// if configuration isn't set in JSON, check Environment vars/flags
			if (vm.VCenterURL == nil) || *vm.VCenterURL == "" {
				if vm.VCenterURL = cmd.Flags().String("vcurl", os.Getenv("VCURL"), "VMware vCenter URL, format https://user:pass@address/sdk [REQD]"); *vm.VCenterURL == "" {
					log.Fatalf("VMware vCenter/vSphere credentials are missing")
				}
			}

			if (vm.DCName == nil) || *vm.DCName == "" {
				if vm.DCName = cmd.Flags().String("datacenter", os.Getenv("VCDATACENTER"), "The name of the Datacenter to host the VM [REQD]"); *vm.DCName == "" {
					log.Printf("No Datacenter was specified, will try to use the default (will cause errors with Linked-Mode)")
				}
			}

			if (vm.DSName == nil) || *vm.DSName == "" {
				if vm.DSName = cmd.Flags().String("datastore", os.Getenv("VCDATASTORE"), "The name of the DataStore to host the VM [REQD]"); *vm.DSName == "" {
					log.Fatalf("A VMware vCenter datastore is required for provisioning")
				}
			}

			if (vm.NetworkName == nil) || *vm.NetworkName == "" {
				if vm.NetworkName = cmd.Flags().String("network", os.Getenv("VCNETWORK"), "The network label the VM will use [REQD]"); *vm.NetworkName == "" {
					log.Fatalf("Specify a Network to connect to")
				}
			}

			if (vm.VSphereHost == nil) || *vm.VSphereHost == "" {
				if vm.VSphereHost = cmd.Flags().String("hostname", os.Getenv("VCHOST"), "The server that will run the VM [REQD]"); *vm.VSphereHost == "" {
					log.Fatalf("A Host inside of vCenter/vSphere is required to provision on for VM capacity")
				}
			}

			// Ideally these should be populated as they're needed for a lot of the tasks.
			if (vm.VMTemplateAuth.Username == nil) || *vm.VMTemplateAuth.Username == "" {
				if vm.VMTemplateAuth.Username = cmd.Flags().String("templateUser", os.Getenv("VMUSER"), "A created user inside of the VM template"); *vm.VMTemplateAuth.Username == "" {
					log.Printf("No Username for inside of the Guest OS was specified, somethings may fail")
				}
			}

			if (vm.VMTemplateAuth.Password == nil) || *vm.VMTemplateAuth.Password == "" {
				if vm.VMTemplateAuth.Password = cmd.Flags().String("templatePass", os.Getenv("VMPASS"), "The password for the specified user inside the VM template"); *vm.VMTemplateAuth.Username == "" {
					log.Printf("No Password for inside of the Guest OS was specified, somethings may fail")
				}
			}

			if *vm.VCenterURL == "" || *vm.DSName == "" || *vm.VSphereHost == "" || len(args) != 1 {
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

		if task != nil {
			log.Printf("Beginning Task [%s]: %s", task.Name, task.Note)

			newVM, err := provision(ctx, client, *vm, task.Task.InputTemplate, task.Task.OutputName)

			if err != nil {
				log.Printf("Provisioning has failed =>")
				log.Fatalf("%v", err)
			}

			auth := &types.NamePasswordAuthentication{
				Username: *vm.VMTemplateAuth.Username,
				Password: *vm.VMTemplateAuth.Password,
			}

			runCommands(ctx, client, newVM, auth, task)
			if task.Task.OutputType == "Template" {
				log.Printf("Provisioning tasks have completed, powering down Virtual Machine (120 second Timeout)")

				err = newVM.ShutdownGuest(ctx)
				if err != nil {
					log.Printf("Power Off task failed =>")
					log.Fatalf("%v", err)
				}
				for i := 1; i <= 60; i++ {
					state, err := newVM.PowerState(ctx)
					if err != nil {
						log.Fatalf("%v", err)
					}
					if state != types.VirtualMachinePowerStatePoweredOff {
						fmt.Printf(".")
					} else {
						fmt.Printf("\n")
						break
					}
					time.Sleep(2 * time.Second)
				}
				err = newVM.MarkAsTemplate(ctx)
				if err != nil {
					log.Printf("Marking as Template has failed =>")
					log.Fatalf("%v", err)
				}
			}
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
				var err error
				var pid int64
				fmt.Printf("%s\n", cmd.CMDkey)
				if cmd.CMDkey != "" {
					log.Printf("Executing command from key [%s]", cmd.CMDkey)
					execKey := cmdResults[cmd.CMDkey]
					fmt.Printf("%s\n", execKey)

					pid, err = vmExec(ctx, client, vm, auth, execKey, "")
				} else {
					pid, err = vmExec(ctx, client, vm, auth, cmd.CMDPath, cmd.CMDArgs)
				}
				if err != nil {
					log.Fatalf("%v", err)
				}
				if cmd.CMDIgnore == false {
					err = watchPid(ctx, client, vm, auth, []int64{pid})
					if err != nil {
						log.Fatalf("%v", err)
					}
				}
			case "download":
				err := vmDownloadFile(ctx, client, vm, auth, cmd.CMDPath, cmd.CMDresultKey, cmd.CMDDelete)
				if err != nil {
					fmt.Printf("Error\n")
					log.Fatalf("%v", err)
				}
			}
			// Execute the command on the Virtual Machine

		}
	}
	ResetCounter()
}

func vmExec(ctx context.Context, client *govmomi.Client, vm *object.VirtualMachine, auth *types.NamePasswordAuthentication, path string, args string) (int64, error) {
	o := guest.NewOperationsManager(client.Client, vm.Reference())
	pm, _ := o.ProcessManager(ctx)

	newPath := "/bin/sudo"
	newArgs := fmt.Sprintf("-u root %s %s", path, args)
	fmt.Printf("%s\n", newArgs)
	cmdSpec := types.GuestProgramSpec{
		ProgramPath: newPath,
		Arguments:   newArgs,
	}

	pid, err := pm.StartProgram(ctx, auth, &cmdSpec)
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func readEnv(ctx context.Context, client *govmomi.Client, vm *object.VirtualMachine, auth *types.NamePasswordAuthentication, path string, args string) error {
	o := guest.NewOperationsManager(client.Client, vm.Reference())
	pm, _ := o.ProcessManager(ctx)

	test, err := pm.ReadEnvironmentVariable(ctx, auth, []string{"swarm"})
	if err != nil {
		return err
	}
	fmt.Printf("%s", test)
	return nil
}

// This will download a file from the Virtual Machine to the localhost
func vmDownloadFile(ctx context.Context, client *govmomi.Client, vm *object.VirtualMachine, auth *types.NamePasswordAuthentication, path string, key string, deleteonDownload bool) error {
	o := guest.NewOperationsManager(client.Client, vm.Reference())
	fm, _ := o.FileManager(ctx)
	fileDetails, err := fm.InitiateFileTransferFromGuest(ctx, auth, path)
	if err != nil {
		return err
	}

	dl := soap.DefaultDownload

	e, err := client.ParseURL(fileDetails.Url)
	if err != nil {
		return err
	}

	f, _, err := client.Download(e, &dl)
	if err != nil {
		return err
	}
	// This will change to allow us to store contents of the filesystem in memory
	//_, err = io.Copy(os.Stdout, f)

	if key != "" {
		body, err := ioutil.ReadAll(f)
		if err != nil {
			return err
		}
		convertedString := string(body)
		cmdResults[key] = convertedString
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
