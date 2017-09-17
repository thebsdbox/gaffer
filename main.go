package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/guest"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

var cmdResults = map[string]string{}

func main() {
	InitDeployment()
	vm := VMwareConfig() //Pull VMware configuration from JSON

	var vc, dc, ds, nn, vh, gu, gp *string

	cmd := &cobra.Command{
		Use:   "./gaffer deployment.json",
		Short: "This tool uses the native VMware APIs to automate Virtual Machines through the guest tools",
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
				if vm.VCenterURL = vc; *vm.VCenterURL == "" {
					log.Fatalf("VMware vCenter/vSphere credentials are missing")
				}
			}

			if (vm.DCName == nil) || *vm.DCName == "" {
				if vm.DCName = dc; *vm.DCName == "" {
					log.Warnf("No Datacenter was specified, will try to use the default (will cause errors with Linked-Mode)")
				}
			}

			if (vm.DSName == nil) || *vm.DSName == "" {
				if vm.DSName = ds; *vm.DSName == "" {
					log.Fatalf("A VMware vCenter datastore is required for provisioning")
				}
			}

			if (vm.NetworkName == nil) || *vm.NetworkName == "" {
				if vm.NetworkName = nn; *vm.NetworkName == "" {
					log.Fatalf("Specify a Network to connect to")
				}
			}

			if (vm.VSphereHost == nil) || *vm.VSphereHost == "" {
				if vm.VSphereHost = vh; *vm.VSphereHost == "" {
					log.Fatalf("A Host inside of vCenter/vSphere is required to provision on for VM capacity")
				}
			}

			// Ideally these should be populated as they're needed for a lot of the tasks.
			if (vm.VMTemplateAuth.Username == nil) || *vm.VMTemplateAuth.Username == "" {
				if vm.VMTemplateAuth.Username = gu; *vm.VMTemplateAuth.Username == "" {
					log.Warnf("No Username for inside of the Guest OS was specified, somethings may fail")
				}
			}

			if (vm.VMTemplateAuth.Password == nil) || *vm.VMTemplateAuth.Password == "" {
				if vm.VMTemplateAuth.Password = gp; *vm.VMTemplateAuth.Username == "" {
					log.Warnf("No Password for inside of the Guest OS was specified, somethings may fail")
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
			// Iterate through the deployments and tasks
			runTasks(ctx, client)
		},
	}

	vc = cmd.Flags().String("vcurl", os.Getenv("VCURL"), "VMware vCenter URL, format https://user:pass@address/sdk [REQD]")
	dc = cmd.Flags().String("datacenter", os.Getenv("VCDATACENTER"), "The name of the Datacenter to host the VM [REQD]")
	ds = cmd.Flags().String("datastore", os.Getenv("VCDATASTORE"), "The name of the DataStore to host the VM [REQD]")
	nn = cmd.Flags().String("network", os.Getenv("VCNETWORK"), "The network label the VM will use [REQD]")
	vh = cmd.Flags().String("hostname", os.Getenv("VCHOST"), "The server that will run the VM [REQD]")
	gu = cmd.Flags().String("templateUser", os.Getenv("VMUSER"), "A created user inside of the VM template")
	gp = cmd.Flags().String("templatePass", os.Getenv("VMPASS"), "The password for the specified user inside the VM template")

	log.Println("Starting Gaffer")
	err := cmd.Execute()
	if err != nil {
		log.Fatalf("Error parsing the flags")
	}
	log.Println("Gaffer Completed Succesfully")
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
				for i := 1; i <= 120; i++ {
					state, err := newVM.PowerState(ctx)
					if err != nil {
						log.Fatalf("%v", err)
					}
					if state != types.VirtualMachinePowerStatePoweredOff {
						fmt.Printf("\r\033[36mWaiting for\033[m %d Seconds for VM Shutdown", i)
					} else {
						fmt.Printf("\r\033[32mShutdown completed in\033[m %d Seconds        \n", i)
						break
					}
					time.Sleep(1 * time.Second)
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
				if cmd.CMDkey != "" {
					log.Infof("Executing command from key [%s]", cmd.CMDkey)
					execKey := cmdResults[cmd.CMDkey]
					pid, err = vmExec(ctx, client, vm, auth, execKey, cmd.CMDUser)
				} else {
					pid, err = vmExec(ctx, client, vm, auth, cmd.CMD, cmd.CMDUser)
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
				err := vmDownloadFile(ctx, client, vm, auth, cmd.CMDFilePath, cmd.CMDresultKey, cmd.CMDDelete)
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

func vmExec(ctx context.Context, client *govmomi.Client, vm *object.VirtualMachine, auth *types.NamePasswordAuthentication, command string, user string) (int64, error) {
	o := guest.NewOperationsManager(client.Client, vm.Reference())
	pm, _ := o.ProcessManager(ctx)

	sudoPath := "/bin/sudo" //TODO: This should perhaps be configurable incase some Distro has sudo in a weird place.

	// Add User to the built command
	var builtPath string
	if user != "" {
		builtPath = fmt.Sprintf("-n -u %s %s", user, command)
	} else {
		builtPath = fmt.Sprintf("-n %s", command)
	}

	cmdSpec := types.GuestProgramSpec{
		ProgramPath: sudoPath,
		Arguments:   builtPath,
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
		log.Printf("Watching process [%d] cmd [%s]", process[0].Pid, process[0].CmdLine)
	} else {
		log.Fatalf("Process couldn't be found running")
	}

	// Counter if VMtools loses a previously watched process
	processTimeout := 0
	var counter int
	for {
		time.Sleep(1 * time.Second)
		process, err = pm.ListProcesses(ctx, auth, pid)

		if err != nil {
			return err
		}
		// Watch Process
		if process[0].EndTime == nil {
			fmt.Printf("\r\033[36mWatching for\033[m %d Seconds", counter)
			counter++
		} else {
			if process[0].ExitCode != 0 {
				fmt.Printf("\n")
				log.Println("Return code was not zero, please investigate logs on the Virtual Machine")
				break
			} else {
				fmt.Printf("\r\033[32mTask completed in\033[m %d Seconds\n", counter)
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
