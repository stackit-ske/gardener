// Copyright 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

const (
	// VPNTunnel dictates that VPN is used as a tunnel between seed and shoot networks.
	VPNTunnel string = "vpn-shoot"

	// KubeAPIServerPrefix is a constant for a prefix used for the KubeAPIServer instance which uses a TLS certificate from a trusted origin.
	KubeAPIServerPrefix = "api"

	// PlutonoUsersPrefix is a constant for a prefix used for the users Plutono instance.
	PlutonoUsersPrefix = "gu"

	// PrometheusPrefix is a constant for a prefix used for the Prometheus instance.
	PrometheusPrefix = "p"

	// AlertManagerPrefix is a constant for a prefix used for the AlertManager instance.
	AlertManagerPrefix = "au"

	// ValiPrefix is a constant for a prefix used for the Vali instance.
	ValiPrefix = "v"

	// ManagedResourceShootCoreName is the name of the shoot core managed resource.
	ManagedResourceShootCoreName = "shoot-core"
	// ManagedResourceAddonsName is the name of the addons managed resource.
	ManagedResourceAddonsName = "addons"

	// ShootDNSIngressName is a constant for the DNS resources used for the shoot ingress addon.
	ShootDNSIngressName = "ingress"
)
