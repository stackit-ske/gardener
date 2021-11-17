#!/bin/bash

FILE=/etc/containerd/config.toml
rm $FILE
if [ ! -f "$FILE" ]; then
  mkdir -p /etc/containerd
  echo 'imports = ["/etc/containerd/runtime_*.toml"]' > "$FILE"
  containerd config default | sed "s#imports = \[\]##g" >> "$FILE"
fi

# use injected image as sandbox image
sandbox_image_line="$(grep sandbox_image $FILE | sed -e 's/^[ ]*//')"
pause_image={{ .pauseContainerImage }}
sed -i  "s|$sandbox_image_line|sandbox_image = \"$pause_image\"|g" $FILE

BIN_PATH={{ .binaryPath }}
mkdir -p $BIN_PATH

ENV_FILE=/etc/systemd/system/containerd.service.d/30-env_config.conf
if [ ! -f "$ENV_FILE" ]; then
  cat <<EOF | tee $ENV_FILE
[Service]
Environment="PATH=$BIN_PATH:$PATH"
EOF
  systemctl daemon-reload
fi
