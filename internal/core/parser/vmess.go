package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type vmessConfig struct {
	Raw      string `json:"-"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	ID       string `json:"id"`
	AlterID  int    `json:"alterId"`
	Security string `json:"security"`
	Network  string `json:"network"`
	Type     string `json:"type"`
	Host     string `json:"host,omitempty"`
	Path     string `json:"path,omitempty"`
	TLS      string `json:"tls,omitempty"`
	SNI      string `json:"sni,omitempty"`
	Remark   string `json:"remark,omitempty"`
}

type vmessLinkConfig struct {
	V    string `json:"v"`
	PS   string `json:"ps"`
	Add  string `json:"add"`
	Port string `json:"port"`
	ID   string `json:"id"`
	Aid  string `json:"aid"`
	Scy  string `json:"scy"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
}

type vmessJSONUser struct {
	ID       string `json:"id"`
	AlterID  int    `json:"alterId"`
	Security string `json:"security"`
}

type vmessJSONVnext struct {
	Address string          `json:"address"`
	Port    int             `json:"port"`
	Users   []vmessJSONUser `json:"users"`
}

type vmessJSONStreamSettings struct {
	Network      string                 `json:"network"`
	Security     string                 `json:"security,omitempty"`
	WSSettings   map[string]interface{} `json:"wsSettings,omitempty"`
	TCPSettings  map[string]interface{} `json:"tcpSettings,omitempty"`
	KCPSettings  map[string]interface{} `json:"kcpSettings,omitempty"`
	HTTPSettings map[string]interface{} `json:"httpSettings,omitempty"`
	QUICSettings map[string]interface{} `json:"quicSettings,omitempty"`
	GRPCSettings map[string]interface{} `json:"grpcSettings,omitempty"`
	TLSSettings  map[string]interface{} `json:"tlsSettings,omitempty"`
}

type vmessJSONSettings struct {
	Vnext []vmessJSONVnext `json:"vnext"`
}

func (c *vmessConfig) MarshalJSON() ([]byte, error) {
	user := vmessJSONUser{
		ID:       c.ID,
		AlterID:  c.AlterID,
		Security: c.Security,
	}

	vnext := vmessJSONVnext{
		Address: c.Server,
		Port:    c.Port,
		Users:   []vmessJSONUser{user},
	}

	streamSettings := vmessJSONStreamSettings{
		Network: c.Network,
	}

	switch c.Network {
	case "ws":
		wsSettings := make(map[string]interface{})
		if c.Path != "" {
			wsSettings["path"] = c.Path
		}
		if c.Host != "" {
			wsSettings["host"] = c.Host
		}
		if len(wsSettings) > 0 {
			streamSettings.WSSettings = wsSettings
		}
	case "tcp":
		if c.Type == "http" {
			tcpSettings := make(map[string]interface{})
			header := make(map[string]interface{})
			header["type"] = "http"
			if c.Host != "" {
				request := make(map[string]interface{})
				request["headers"] = map[string][]string{"Host": {c.Host}}
				header["request"] = request
			}
			tcpSettings["header"] = header
			streamSettings.TCPSettings = tcpSettings
		}
	case "kcp":
		kcpSettings := make(map[string]interface{})
		header := make(map[string]interface{})
		header["type"] = c.Type
		kcpSettings["header"] = header
		streamSettings.KCPSettings = kcpSettings
	case "http", "h2":
		httpSettings := make(map[string]interface{})
		if c.Path != "" {
			httpSettings["path"] = c.Path
		}
		if c.Host != "" {
			httpSettings["host"] = []string{c.Host}
		}
		streamSettings.HTTPSettings = httpSettings
	case "quic":
		quicSettings := make(map[string]interface{})
		quicSettings["security"] = c.Host
		quicSettings["key"] = c.Path
		header := make(map[string]interface{})
		header["type"] = c.Type
		quicSettings["header"] = header
		streamSettings.QUICSettings = quicSettings
	case "grpc":
		grpcSettings := make(map[string]interface{})
		grpcSettings["serviceName"] = c.Path
		streamSettings.GRPCSettings = grpcSettings
	}

	if c.TLS == "tls" {
		streamSettings.Security = "tls"
		tlsSettings := make(map[string]interface{})
		if c.SNI != "" {
			tlsSettings["serverName"] = c.SNI
		} else if c.Host != "" {
			tlsSettings["serverName"] = c.Host
		}
		if len(tlsSettings) > 0 {
			streamSettings.TLSSettings = tlsSettings
		}
	}

	outboundConfig := map[string]interface{}{
		"protocol":       "vmess",
		"settings":       vmessJSONSettings{Vnext: []vmessJSONVnext{vnext}},
		"streamSettings": streamSettings,
	}

	return json.Marshal(outboundConfig)
}

func parseVMess(link string) (Config, error) {
	if !strings.HasPrefix(link, "vmess://") {
		return nil, fmt.Errorf("invalid VMess link format")
	}

	parts := strings.Split(link, "://")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid VMess link format")
	}

	// Try multiple base64 decoding methods
	var data []byte
	var err error

	// Try standard base64
	data, err = base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		// Try URL-safe base64
		data, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			// Try raw standard base64
			data, err = base64.RawStdEncoding.DecodeString(parts[1])
			if err != nil {
				// Try raw URL-safe base64
				data, err = base64.RawURLEncoding.DecodeString(parts[1])
				if err != nil {
					return nil, fmt.Errorf("invalid VMess link format: unable to decode base64")
				}
			}
		}
	}

	var linkConfig vmessLinkConfig
	if err := json.Unmarshal(data, &linkConfig); err != nil {
		return nil, fmt.Errorf("invalid VMess link format: %v", err)
	}

	if err := validateAddress(linkConfig.Add); err != nil {
		return nil, fmt.Errorf("invalid VMess link format: %v", err)
	}

	config := &vmessConfig{
		Raw:      link,
		Server:   linkConfig.Add,
		ID:       linkConfig.ID,
		Security: linkConfig.Scy,
		Network:  linkConfig.Net,
		Type:     linkConfig.Type,
		Host:     linkConfig.Host,
		Path:     linkConfig.Path,
		TLS:      linkConfig.TLS,
		SNI:      linkConfig.SNI,
		Remark:   linkConfig.PS,
	}

	if linkConfig.Port != "" {
		port, err := strconv.Atoi(linkConfig.Port)
		if err != nil {
			return nil, fmt.Errorf("invalid port: %v", err)
		}
		config.Port = port
	}

	if linkConfig.Aid != "" {
		aid, err := strconv.Atoi(linkConfig.Aid)
		if err != nil {
			return nil, fmt.Errorf("invalid alter ID: %v", err)
		}
		config.AlterID = aid
	}

	// Set defaults
	if config.Security == "" {
		config.Security = "auto"
	}
	if config.Network == "" {
		config.Network = "tcp"
	}
	if config.Type == "" {
		config.Type = "none"
	}

	// Validation
	if config.Server == "" {
		return nil, fmt.Errorf("server address is required")
	}
	if config.ID == "" {
		return nil, fmt.Errorf("user ID is required")
	}
	if config.Port == 0 {
		return nil, fmt.Errorf("port is required")
	}

	return config, nil
}
