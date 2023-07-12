# Make work with IPv6

# Seed Box

sh create.sh

kubectl patch service/registry -n registry --subresource status -p '{"status":{"loadBalancer":{"ingress":[{"ip":"2600:1900:40d0:90a3:0:2::1"}]}}}'
kubectl patch service/gardener-apiserver-tunnel -n relay --subresource status -p '{"status":{"loadBalancer":{"ingress":[{"ip":"2600:1900:40d0:90a3:0:2::1"}]}}}'
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -sha256 -days 3650 -nodes -subj "/CN=reg.hackathon2.ci.ske.eu01.stackit.cloud/C=XX/ST=StateName/L=CityName/O=CompanyName/OU=CompanySectionName/CN=CommonNameOrHostname"

Add
[plugins."io.containerd.grpc.v1.cri".registry.configs."reg.hackathon.ci.ske.eu01.stackit.cloud".tls]
insecure_skip_verify = true
to /etc/containerd/config.toml

kubectl patch service/nginx-ingress-controller -n garden --subresource status -p '{"status":{"loadBalancer":{"ingress":[{"ip":"2600:1900:40d0:90a3:0:2::2"}]}}}'
kubectl patch service/istio-ingressgateway -n istio-ingress --subresource status -p '{"status":{"loadBalancer":{"ingress":[{"ip":"2600:1900:40d0:90a3:0:2::3"}]}}}'


# DEV BOX

sudo ip6tables -t nat -A POSTROUTING -o "$(ip route show default | awk '{print $5}')" -s fd00:10::/64 -j MASQUERADE
git checkout hackathon-ipv6

example/provider-extensions/garden/cloudprofiles/cloudprofile.yaml
example/provider-extensions/garden/controllerregistrations/calico.yaml
example/provider-extensions/garden/controllerregistrations/gardenlinux.yaml
example/provider-extensions/garden/controllerregistrations/gcp.yaml
example/provider-extensions/garden/controllerregistrations/ubuntu.yaml
example/provider-extensions/garden/controlplane/values.yaml
example/provider-extensions/garden/project/credentials/infrastructure-secrets.yaml
example/provider-extensions/garden/project/credentials/secretbindings.yaml
example/provider-extensions/gardenlet/values.yaml
example/provider-extensions/seed/kubeconfig

make kind-extensions-up IPFAMILY=ipv6

make gardener-extensions-up IPFAMILY=ipv6


cat >/etc/docker/daemon.json <<EOF
{
"insecure-registries": ["https://reg.hackathon2.ci.ske.eu01.stackit.cloud"]
}
EOF

/etc/hosts
::1 ip6-localhost ip6-loopback localhost

# Shoot (done via operatingsystemconfig)

- Node IP needs to be IPv6
- DNS for kubelet needs to be IPv6

INT=$(ip route show default | awk '{print $5}')
LOCAL_IPV6=$(ip a s $INT | awk '$1 == "inet6" {print $2}' |grep -v fe80 |cut -d"/" -f1)

echo "nameserver 2001:4860:4860::8888" > /var/lib/kubelet/resolv.conf
echo 'KUBELET_EXTRA_ARGS= --node-ip="2600:1900:40d0:1ef2:0:43::" --resolv-conf=/var/lib/kubelet/resolv.conf' > /var/lib/kubelet/extra_args

kubectl patch node/shoot--garden--secv6-gcp-z1-655bc-4vg9c --subresource status -p '{"status":{"addresses":[{"address":"2600:1900:40d0:1ef2:0:3c::","type":"InternalIP"}]}}'


## TODO

- Patch network cidr to Shoot networking.nodes
- Remove cloud provider taint from Shoot node (GCP cloud controller tries to reach http://169.254.169.254/computeMetadata/v1/instance/hostname)

# Trash
iptables -t nat -A PREROUTING -i ens4 -p tcp --dport 80 -j REDIRECT --to-port 31537
iptables -t nat -A PREROUTING -i ens4 -p tcp --dport 443 -j REDIRECT --to-port 30200

kubectl patch shoot/secv6 -n garden -p '{"spec":{"networking":{"nodes":"2600:1900:40d0:1ef2:0:b::/64"}}}'

