#!/usr/bin/env bash

INT=$(ip route show default | awk '{print $5}')
LOCAL_IPV6=$(ip a s $INT | awk '$1 == "inet6" {print $2}' |grep -v fe80 |cut -d"/" -f1)
LOCAL_IPV4=$(ip a s $INT | awk '$1 == "inet" {print $2}'|cut -d"/" -f1)
EXTERNAL_IPV4=$(curl -H "Metadata-Flavor:Google" http://169.254.169.254/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip)

echo "nameserver 2001:4860:4860::8844" |sudo tee /var/lib/kubelet/resolv.conf
echo "KUBELET_EXTRA_ARGS=\"--node-ip=$LOCAL_IPV6\" --resolv-conf=/var/lib/kubelet/resolv.conf" |sudo tee /etc/default/kubelet
sudo systemctl restart kubelet

sudo kubeadm init \
    --apiserver-advertise-address "$LOCAL_IPV6" \
    --apiserver-cert-extra-sans "$LOCAL_IPV6,$EXTERNAL_IPV4" \
    --kubernetes-version "1.25.2" \
    --pod-network-cidr "fd00:10:1::/56" \
    --service-cidr "fd00:10:2::/112" \

# Copy kubeconfig
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config

# Remove master taint
kubectl taint nodes $(hostname) node-role.kubernetes.io/control-plane:NoSchedule-
# Apply local path provisioner
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/v0.0.24/deploy/local-path-storage.yaml

kubectl annotate sc local-path storageclass.kubernetes.io/is-default-class="true"