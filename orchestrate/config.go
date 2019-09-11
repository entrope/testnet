package main

import (
	"net/netip"
)

// NOTE: This file does not attempt to support the full Compse schema,
// only the subset that is used by this `orchestrate` utility.

// Compose holds a compose config file.
type Compose struct {
	// Name is an optional name for the configuration.
	Name string `yaml:",omitempty"`

	// Services describes the services in the pod.
	Services map[string]*Service

	// Networks describes the networks in the pod.
	Networks map[string]*Network

	// Config maps config names to details.
	Configs map[string]*ConfigOrSecret
}

// ConfigOrSecret describes some information to be passed to a container.
type ConfigOrSecret struct {
	// File names the file providing the configuration content.
	File string `yaml:",omitempty"`
}

// Extends identifies a base service.
type Extends struct {
	// Service is the name of the base (template) service.
	Service string

	// File is the optional name of the file defining `Service`.
	File string `yaml:",omitempty"`
}

// IPAM defines IP address management settings for a network.
type IPAM struct {
	Config []IPAMConfig
}

// IPAMConfig defines a particular network range to use.
type IPAMConfig struct {
	// Subnet is the subnet to use.
	Subnet netip.Prefix
}

// Network describes a network in the pod.
type Network struct {
	// Attachable indicates whether standalone containers should be
	// allowed to attach to this network.
	Attachable bool

	// EnableIPv6 turns on IPv6 support on the network.
	EnableIPv6 bool `yaml:"enable_ipv6"`

	// Internal indicates whether external connectivity should be
	// disabled.
	Internal bool

	// IPAM specifies the network configuration.
	IPAM IPAM
}

// Service describes a container instance within the application.
type Service struct {
	// Configs lists the configurations used by the service.
	Configs []ServiceConfig

	// ContainerName indicates what container to use for the service.
	ContainerName string `yaml:"container_name,omitempty"`

	// Extends is used to inherit common values so they are not repeated.
	Extends Extends `yaml:",omitempty"`

	// Image names the image used for this service.
	Image string

	// IPv4Address names the IPv4 address to use.
	IPv4Address netip.Addr `yaml:"ipv4_address,omitempty"`

	// IPv6Address names the IPv6 address to use.
	IPv6Address netip.Addr `yaml:"ipv6_address,omitempty"`

	// Networks names the network(s) that this service uses.
	Networks []string

	// PullPolicy is how/when Compose retrieves the image.
	PullPolicy string `yaml:"pull_policy,omitempty"`

	// Sysctls is a list of sysctl values to override.
	Sysctls map[string]string
}

// ServiceConfig describes the "long" syntax for a config.
type ServiceConfig struct {
	// Source names the config within the Compose file.
	Source string

	// Target is the path to mount the config within the container.
	Target string
}
