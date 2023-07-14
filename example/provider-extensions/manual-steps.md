# Make work with IPv6

## [Seed Box] 1. Create K8s Cluster

Create k8s cluster with this script:

```bash
example/provider-extensions/seed/create-k8s.sh
```

Install Calico CNI

```bash
kubectl apply -k example/gardener-local/calico/ipv6
```

## [DEV BOX] 2. Prepare

```bash 
sudo ip6tables -t nat -A POSTROUTING -o "$(ip route show default | awk '{print $5}')" -s fd00:10::/64 -j MASQUERADE
sed -i "s#::1#::1 localhost#g" hosts /etc/hosts
```

```
git checkout hackathon-ipv6
```

## [DEV BOX] 3. Populate value files

- example/provider-extensions/garden/cloudprofiles/cloudprofile.yaml
- example/provider-extensions/garden/controllerregistrations/calico.yaml
- example/provider-extensions/garden/controllerregistrations/gardenlinux.yaml
- example/provider-extensions/garden/controllerregistrations/gcp.yaml
- example/provider-extensions/garden/controllerregistrations/ubuntu.yaml
- example/provider-extensions/garden/project/credentials/infrastructure-secrets.yaml
- example/provider-extensions/garden/project/credentials/secretbindings.yaml
- example/provider-extensions/garden/controlplane/values.yaml
- example/provider-extensions/gardenlet/values.yaml

Set DNS for your setup in `example/provider-extensions/garden/controlplane/values.yaml`

Review Gardenlet values in `example/provider-extensions/gardenlet/values.yaml`:

- DNS secret ref
- spec.network.nodes: must be local IPv6 address of Seed box

## [DEV BOX] 4. Copy kubeconfig from seed box

Copy kubeconfig to `example/provider-extensions/seed/kubeconfig`


## [Seed Box] 5. Generate certificate

- If you are not using a Shoot as Seed, you have to generate a self signed certificate for the registry
  - You can use: `openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -sha256 -days 3650 -nodes -subj "/CN=${REGISTRY_URL}/C=XX/ST=StateName/L=CityName/O=CompanyName/OU=CompanySectionName/CN=CommonNameOrHostname"`
  - use this command to create the needed secret in seed: `kubectl create secret tls tls -n registry --cert=cert.pem --key=key.pem`


## [Dev Box] 6a. Configure Docker

Run as root on Dev box:
```bash
cat >/etc/docker/daemon.json <<EOF
{
"insecure-registries": ["https://reg.hackathon3.ci.ske.eu01.stackit.cloud"]
}
EOF
```

## [Seed Box] 6b.Add insecure registry

Add
```ini
[plugins."io.containerd.grpc.v1.cri".registry.configs."reg.hackathon3.ci.ske.eu01.stackit.cloud".tls]
insecure_skip_verify = true
```
to `/etc/containerd/config.toml`

## [DEV BOX] 7. Start local setup with extensions

Run kind up
```bash
make kind-extensions-up IPFAMILY=ipv6
```

Prepare registyr and relay url beforehand
```bash
make gardener-extensions-up IPFAMILY=ipv6
```

Pause after Registry and Relay Domain, goto next step

## 8. Set DNS entries

Get two addresses from IPv6 prefix and set DNS entries corresponding. Sample:

- reg.hackathon3.ci.ske.eu01.stackit.cloud -> AAAA 2600:1900:40d0:1ef2:0:1::1
- relay.hackathon3.ci.ske.eu01.stackit.cloud -> AAAA 2600:1900:40d0:1ef2:0:1::2

Unpause make

## [Seed Box] 9. Patch services


kubectl patch service/registry -n registry --subresource status -p '{"status":{"loadBalancer":{"ingress":[{"ip":"2600:1900:40d0:1ef2:0:1::1"}]}}}'
kubectl patch service/gardener-apiserver-tunnel -n relay --subresource status -p '{"status":{"loadBalancer":{"ingress":[{"ip":"2600:1900:40d0:1ef2:0:1::2"}]}}}'

Add label to node object `topology.kubernetes.io/zone: europe-west3-c`

After services available:
kubectl patch service/nginx-ingress-controller -n garden --subresource status -p '{"status":{"loadBalancer":{"ingress":[{"ip":"2600:1900:40d0:1ef2:0:1::3"}]}}}'
kubectl patch service/istio-ingressgateway -n istio-ingress --subresource status -p '{"status":{"loadBalancer":{"ingress":[{"ip":"2600:1900:40d0:1ef2:0:1::4"}]}}}'

## [Dev Box] 10. Deploy shoot

kubectl apply -f example/provider-extensions/shoot-ipv6.yaml

## Access 

- Patch network cidr to Shoot networking.nodes
    - `kubectl patch shoot/hackathon3 -n garden -p '{"spec":{"networking":{"nodes":"2600:1900:40d0:e06d:0:0:0:0/64"}}}'`
- Remove cloud provider taint from Shoot node (GCP cloud controller tries to reach http://169.254.169.254/computeMetadata/v1/instance/hostname)


# Shoot (done via operatingsystemconfig)

- Node IP needs to be IPv6
- DNS for kubelet needs to be IPv6

INT=$(ip route show default | awk '{print $5}')
LOCAL_IPV6=$(ip a s $INT | awk '$1 == "inet6" {print $2}' |grep -v fe80 |cut -d"/" -f1)

echo "nameserver 2001:4860:4860::8888" > /var/lib/kubelet/resolv.conf
echo 'KUBELET_EXTRA_ARGS= --node-ip="2600:1900:40d0:1ef2:0:43::" --resolv-conf=/var/lib/kubelet/resolv.conf' > /var/lib/kubelet/extra_args

kubectl patch node/shoot--garden--secv6-gcp-z1-655bc-4vg9c --subresource status -p '{"status":{"addresses":[{"address":"2600:1900:40d0:1ef2:0:3c::","type":"InternalIP"}]}}'




# Trash
iptables -t nat -A PREROUTING -i ens4 -p tcp --dport 80 -j REDIRECT --to-port 31537
iptables -t nat -A PREROUTING -i ens4 -p tcp --dport 443 -j REDIRECT --to-port 30200



