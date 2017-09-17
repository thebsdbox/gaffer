# Gaffer

A tool to make use of the VMware APIs to automate the provisioning of virtual machines.

There are two provided examples that detail the usage, one will update a CentOS (latest release) VMware
template to support the new release of Docker-CE. The second will create a swarm master and add two 
swarm workers to the cluster. These two examples cover a lot of the usage and command structure for the 
build plans.

**Windows support** Still to be tested.

## Building
Clone the repository and then:

`go get -u`

`go build`

```
$ ./gaffer 
INFO[0000] Starting Gaffer                              
Usage:
  ./gaffer deployment.json [flags]

Flags:
      --datacenter string     The name of the Datacenter to host the VM [REQD]
      --datastore string      The name of the DataStore to host the VM [REQD]
  -h, --help                  help for dockerVM
      --hostname string       The server that will run the VM [REQD]
      --network string        The network label the VM will use [REQD]
      --templatePass string   The password for the specified user inside the VM template
      --templateUser string   A created user inside of the VM template
      --vcurl string          VMware vCenter URL, format https://user:pass@address/sdk [REQD]
```

## Usage

VMware vCenter configuration details can be passed in three ways, either in the JSON/Environment variables or through flags to the executable

```
./gaffer ./examples/Docker-CE-Template.json

INFO[0000] Starting Gaffer                              
INFO[0000] Finished parsing [Docker-CE-on-CentOS], [1] tasks will be deployed 
WARN[0000] No Datacenter was specified, will try to use the default (will cause errors with Linked-Mode) 
INFO[0000] Beginning Task [Docker Template]: Build new template for CentOS 
INFO[0000] Cloning a New Virtual Machine                
INFO[0048] Modifying Networking backend                 
INFO[0048] Waiting for VMware Tools and Network connectivity... 
INFO[0100] New Virtual Machine has started with IP [10.0.0.3] 
INFO[0100] 20 commands will be executed.                
INFO[0101] Task: Upgrade all packages (except VMware Tools) 
INFO[0102] Watching process [1369] cmd ["/bin/sudo" -n -u root /bin/yum upgrade --exclude=open-vm-tools -y > /tmp/ce-yum-upgrade.log] 
Task completed in 2 Seconds
INFO[0105] Task: Remove any pre-existing Docker Installation 
INFO[0105] Watching process [1387] cmd ["/bin/sudo" -n -u root /bin/yum remove docker docker-common docker-selinux docker-engine] 
Task completed in 1 Seconds
INFO[0107] Task: Install Docker-CE Supporting tools     
INFO[0108] Watching process [1398] cmd ["/bin/sudo" -n -u root /bin/yum install -y yum-utils device-mapper-persistent-data lvm2 -y > /tmp/ce-docker-deps.log] 
Task completed in 2 Seconds
{...}
INFO[0187] Provisioning tasks have completed, powering down Virtual Machine (120 second Timeout) 
Shutdown completed in 4 Secondsutdown
INFO[0190] Gaffer Completed Succesfully     

```
