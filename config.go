package main

type Config struct {
	Volumes    []string          `json:"volumes"`
	Privileged bool              `json:"privileged"`
	Network    NetworkConfig     `json:"network"`
	RemoteEnv  map[string]string `json:"remoteEnv"`
	Features   map[string]any    `json:"features"`
}

type NetworkConfig struct {
	AllowAll     bool     `json:"allowAll"`
	AllowDomains []string `json:"allowDomains"`
	AllowIps     []string `json:"allowIps"`
	ExtraHosts   []string `json:"extraHosts"`
}

type SetupData struct {
	Config                Config
	InstallScriptContents string
}
