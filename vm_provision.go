package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

func vCenterLogin(ctx context.Context, vm VMConfig) (*govmomi.Client, error) {
	// Parse URL from string
	u, err := url.Parse(*vm.VCenterURL)
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

func provision(ctx context.Context, client *govmomi.Client, vm VMConfig, inputTemplate string, outputName string) (*object.VirtualMachine, error) {

	f := find.NewFinder(client.Client, true)

	// Find one and only datacenter, not sure how VMware linked mode will work
	dc, err := f.DatacenterOrDefault(ctx, *vm.DCName)
	if err != nil {
		return nil, fmt.Errorf("No Datacenter instance could be found inside of vCenter %v", err)
	}

	// Make future calls local to this datacenter
	f.SetDatacenter(dc)

	// Find Datastore/Network
	datastore, err := f.DatastoreOrDefault(ctx, *vm.DSName)
	if err != nil {
		return nil, fmt.Errorf("Datastore [%s], could not be found", *vm.DSName)
	}

	dcFolders, err := dc.Folders(ctx)
	if err != nil {
		return nil, fmt.Errorf("Error locating default datacenter folder")
	}

	// Set the host that the VM will be created on
	hostSystem, err := f.HostSystemOrDefault(ctx, *vm.VSphereHost)
	if err != nil {
		return nil, fmt.Errorf("vSphere host [%s], could not be found", *vm.VSphereHost)
	}

	// Find the resource pool attached to this host
	resourcePool, err := hostSystem.ResourcePool(ctx)
	if err != nil {
		return nil, fmt.Errorf("Error locating default resource pool")
	}

	// Use finder for VM template
	vmTemplate, err := f.VirtualMachine(ctx, inputTemplate)
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

	task, err := vmObj.Clone(ctx, dcFolders.VmFolder, outputName, cisp)
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

	newNet, err := f.NetworkOrDefault(ctx, *vm.NetworkName)
	if err != nil {
		log.Fatalf("Network [%s], could not be found", *vm.NetworkName)
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
