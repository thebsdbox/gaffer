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

	cmd := &cobra.Command{
		Use:   "dockerVM <flags> deployment.json",
		Short: "This will take an existing VMware template (RHEL/CentOS (today)), update and prepare it for Docker-CE",
		Run: func(cmd *cobra.Command, args []string) {
			// Use the only argument
			err := OpenFile(args[0])
			if err != nil {
				log.Fatalf("%v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			client, err := vCenterLogin(ctx, vm)
			if err != nil {
				log.Fatalf("%v", err)
			}

			log.Printf("Building an updated Image with Docker-CE")
			newVM, provisionError := provision(ctx, client, vm)

			if provisionError != nil {
				log.Printf("Provisioning has failed =>")
				log.Fatalf("%v", provisionError)
			}

			auth := &types.NamePasswordAuthentication{
			}
		},
	}

	log.Println("Starting Docker VMware deployment")
	err := cmd.Execute()
	if err != nil {
		log.Fatalf("Error parsing the flags")
	}

}


	}

	}
}

	for i := 0; i < cmdCount; i++ {
		// if cmd == nil then no more commands to run
		if cmd != nil {
			if cmd.CMDNote != "" { // If the command has a note, then print it out
				log.Printf("Task: %s", cmd.CMDNote)
			}
			}
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
