# dockerVM

A tiny PoC to ok at automating the process of taking pre-existing VMware templates, attempting to bring them to a standard that
will allow the deployment of Docker (CE Currently).

## Building
Clone the repository and then:

`go get -u`

`go build`

```
./dockerVM 
2017/08/15 19:09:51 Starting Docker VMware deployment
Usage:
  dockerVM <flags> [flags]

Flags:
      --datacenter string     The name of the Datacenter to host the VM [REQD]
      --datastore string      The name of the DataStore to host the VM [REQD]
  -h, --help                  help for dockerVM
      --hostname string       The server that will run the VM [REQD]
      --network string        The network label the VM will use [REQD]
      --template string       The name of a template that be used for a new VM [REQD]
      --templatePass string   The password for the specified user inside the VM template
      --templateUser string   A created user inside of the VM template
      --vcurl string          VMware vCenter URL, format https://user:pass@address/sdk [REQD]
```

## Usage
```
./dockerVM --vcurl https://U:P@vCenterHost/sdk \
--templateUser root \
--templatePass password \
--network="LabNetwork" \
--datastore vSphereNFS \
--datacenter="Home Lab" \
--template=centos7-tmpl \
--hostname esxi01.lab

2017/08/15 18:52:34 Starting Docker VMware deployment
2017/08/15 18:52:34 Building an updated Image with Docker-CE
2017/08/15 18:52:34 Cloning a New Virtual Machine
2017/08/15 18:52:55 Modifying Networking backend
2017/08/15 18:52:55 Waiting for VMware Tools and Network connectivity...
2017/08/15 18:53:46 New Virtual Machine has started with IP [192.168.0.71]
2017/08/15 18:53:47 Watching process [2351] cmd ["/usr/sbin/setenforce" 0]
2017/08/15 18:53:57 Process completed Successfully
2017/08/15 18:53:57 Watching process [2362] cmd ["/bin/yum" upgrade --exclude=open-vm-tools -y > /tmp/ce-yum-upgrade.log]
...............................................
2017/08/15 19:02:04 Process completed Successfully
2017/08/15 19:02:05 Watching process [28312] cmd ["/usr/sbin/setenforce" 1]
2017/08/15 19:02:15 Process completed Successfully
2017/08/15 19:02:15 Watching process [28323] cmd ["/bin/yum" remove docker docker-common docker-selinux docker-engine]
2017/08/15 19:02:25 Process completed Successfully
2017/08/15 19:02:25 Watching process [28334] cmd ["/bin/yum" install -y yum-utils device-mapper-persistent-data lvm2 -y > /tmp/ce-docker-deps.log]
2017/08/15 19:02:36 Process completed Successfully
2017/08/15 19:02:36 Watching process [28354] cmd ["/usr/bin/yum-config-manager" --add-repo https://download.docker.com/linux/centos/docker-ce.repo]
2017/08/15 19:02:46 Process completed Successfully
2017/08/15 19:02:46 Watching process [28366] cmd ["/bin/yum" -y makecache fast]
2017/08/15 19:02:56 Process completed Successfully
2017/08/15 19:02:57 Watching process [28383] cmd ["/bin/yum" -y install docker-ce > /tmp/ce-docker-install.log]
......
2017/08/15 19:04:09 Process completed Successfully
```
