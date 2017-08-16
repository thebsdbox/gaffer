package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/guest"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

type vmConfig struct {
	vCenterURL     *string
	dcName         *string
	dsName         *string
	networkName    *string
	vSphereHost    *string
	template       *string
	vmTemplateAuth struct {
		username *string
		password *string
	}
}

func main() {
	var vm vmConfig

	cmd := &cobra.Command{
		Use:   "dockerVM <flags> deployment.json",
		Short: "This will take an existing VMware template (RHEL/CentOS (today)), update and prepare it for Docker-CE",
		Run: func(cmd *cobra.Command, args []string) {
			if *vm.vCenterURL == "" || *vm.dcName == "" || *vm.dsName == "" || *vm.template == "" || *vm.vSphereHost == "" || len(args) != 1 {
				cmd.Usage()
				os.Exit(1)
			}

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
				Username: *vm.vmTemplateAuth.username,
				Password: *vm.vmTemplateAuth.password,
			}

			runCommands(ctx, client, newVM, auth)
		},
	}

	vm.vCenterURL = cmd.Flags().String("vcurl", os.Getenv("VCURL"), "VMware vCenter URL, format https://user:pass@address/sdk [REQD]")
	vm.dcName = cmd.Flags().String("datacenter", os.Getenv("VCDATACENTER"), "The name of the Datacenter to host the VM [REQD]")
	vm.dsName = cmd.Flags().String("datastore", os.Getenv("VCDATASTORE"), "The name of the DataStore to host the VM [REQD]")
	vm.networkName = cmd.Flags().String("network", os.Getenv("VCNETWORK"), "The network label the VM will use [REQD]")
	vm.vSphereHost = cmd.Flags().String("hostname", os.Getenv("VCHOST"), "The server that will run the VM [REQD]")
	vm.template = cmd.Flags().String("template", os.Getenv("VCTEMPLATE"), "The name of a template that be used for a new VM [REQD]")
	vm.vmTemplateAuth.username = cmd.Flags().String("templateUser", os.Getenv("VMUSER"), "A created user inside of the VM template")
	vm.vmTemplateAuth.password = cmd.Flags().String("templatePass", os.Getenv("VMPASS"), "The password for the specified user inside the VM template")
	log.Println("Starting Docker VMware deployment")
	err := cmd.Execute()
	if err != nil {
		log.Fatalf("Error parsing the flags")
	}

}

func vCenterLogin(ctx context.Context, vm vmConfig) (*govmomi.Client, error) {
	// Parse URL from string
	u, err := url.Parse(*vm.vCenterURL)
	if err != nil {
		return nil, errors.New("URL can't be parsed, ensure it is https://username:password/<address>/sdk")
	}

	// Connect and log in to ESX or vCenter
	client, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		return nil, fmt.Errorf("Error logging into vCenter, check address and credentials\nClient Error: %v", err)
	}
	return client, nil
}

func provision(ctx context.Context, client *govmomi.Client, vm vmConfig) (*object.VirtualMachine, error) {

	f := find.NewFinder(client.Client, true)

	// Find one and only datacenter, not sure how VMware linked mode will work
	dc, err := f.DatacenterOrDefault(ctx, *vm.dcName)
	if err != nil {
		return nil, fmt.Errorf("No Datacenter instance could be found inside of vCenter %v", err)
	}

	// Make future calls local to this datacenter
	f.SetDatacenter(dc)

	// Find Datastore/Network
	datastore, err := f.DatastoreOrDefault(ctx, *vm.dsName)
	if err != nil {
		return nil, fmt.Errorf("Datastore [%s], could not be found", *vm.dsName)
	}

	dcFolders, err := dc.Folders(ctx)
	if err != nil {
		return nil, fmt.Errorf("Error locating default datacenter folder")
	}

	// Set the host that the VM will be created on
	hostSystem, err := f.HostSystemOrDefault(ctx, *vm.vSphereHost)
	if err != nil {
		return nil, fmt.Errorf("vSphere host [%s], could not be found", *vm.vSphereHost)
	}

	// Find the resource pool attached to this host
	resourcePool, err := hostSystem.ResourcePool(ctx)
	if err != nil {
		return nil, fmt.Errorf("Error locating default resource pool")
	}

	// Use finder for VM template
	vmTemplate, err := f.VirtualMachine(ctx, *vm.template)
	if err != nil {
		return nil, err
	}

	pool := resourcePool.Reference()
	host := hostSystem.Reference()
	ds := datastore.Reference()

	// TODO - Allow modifiable relocateSpec for other DataStores
	relocateSpec := types.VirtualMachineRelocateSpec{
		Pool:      &pool,
		Host:      &host,
		Datastore: &ds,
	}

	// The only change we make to the Template Spec, is the config sha and group name
	spec := types.VirtualMachineConfigSpec{
		Annotation: "Built by Docker EE for VMware",
	}

	// Changes can be to spec or relocateSpec
	cisp := types.VirtualMachineCloneSpec{
		Config:   &spec,
		Location: relocateSpec,
		Template: false,
		PowerOn:  true,
	}
	log.Printf("Cloning a New Virtual Machine")
	vmObj := object.NewVirtualMachine(client.Client, vmTemplate.Reference())

	task, err := vmObj.Clone(ctx, dcFolders.VmFolder, "EE-Template", cisp)
	if err != nil {
		return nil, errors.New("Creating new VM failed, more detail can be found in vCenter tasks")
	}

	info, err := task.WaitForResult(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("Creating new VM failed\n%v", err)
	}

	if info.Error != nil {
		return nil, fmt.Errorf("Clone task finished with error: %s", info.Error)
	}

	clonedVM := object.NewVirtualMachine(client.Client, info.Result.(types.ManagedObjectReference))

	devices, _ := clonedVM.Device(ctx)

	net := devices.Find("ethernet-0")
	if net == nil {
		return nil, fmt.Errorf("Ethernet device does not exist on Template")
	}
	currentBacking := net.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()

	newNet, err := f.NetworkOrDefault(ctx, *vm.networkName)
	if err != nil {
		log.Fatalf("Network [%s], could not be found", *vm.networkName)
	}

	backing, err := newNet.EthernetCardBackingInfo(ctx)
	if err != nil {
		log.Fatalf("Unable to determine vCenter network backend\n%v", err)
	}

	netDev, err := object.EthernetCardTypes().CreateEthernetCard("vmxnet3", backing)
	if err != nil {
		log.Fatalf("Unable to create vmxnet3 network interface\n%v", err)
	}

	newBacking := netDev.(types.BaseVirtualEthernetCard).GetVirtualEthernetCard()

	currentBacking.Backing = newBacking.Backing
	log.Printf("Modifying Networking backend")
	clonedVM.EditDevice(ctx, net)

	log.Printf("Waiting for VMware Tools and Network connectivity...")
	guestIP, err := clonedVM.WaitForIP(ctx)
	if err != nil {
		return nil, err
	}

	log.Printf("New Virtual Machine has started with IP [%s]", guestIP)
	return clonedVM, nil
}

func runCommands(ctx context.Context, client *govmomi.Client, vm *object.VirtualMachine, auth *types.NamePasswordAuthentication) {
	cmdCount := CommandCount()
	for i := 0; i < cmdCount; i++ {
		cmd := NextCommand()
		// if cmd == nil then no more commands to run
		if cmd != nil {
			if cmd.CMDNote != "" { // If the command has a note, then print it out
				log.Printf("Task: %s", cmd.CMDNote)
			}
			// Execute the command on the Virtual Machine
			pid, err := vmExec(ctx, client, vm, auth, cmd.CMDPath, cmd.CMDArgs)
			if err != nil {
				log.Fatalf("%v", err)
			}
			if cmd.CMDWatch == true {
				watchPid(ctx, client, vm, auth, []int64{pid})
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
