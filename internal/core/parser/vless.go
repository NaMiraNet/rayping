package parser

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type vlessConfig struct {
	Raw         string `json:"-"`
	Server      string `json:"server"`
	Port        int    `json:"port"`
	ID          string `json:"id"`
	Encryption  string `json:"encryption"`
	Flow        string `json:"flow,omitempty"`
	Security    string `json:"security,omitempty"`
	SNI         string `json:"sni,omitempty"`
	ALPN        string `json:"alpn,omitempty"`
	Network     string `json:"network"`
	Type        string `json:"type,omitempty"`
	Host        string `json:"host,omitempty"`
	Path        string `json:"path,omitempty"`
	Mode        string `json:"mode,omitempty"`
	Authority   string `json:"authority,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	Remark      string `json:"remark,omitempty"`
}

type vlessJSONUser struct {
	ID         string `json:"id"`
	Encryption string `json:"encryption"`
	Flow       string `json:"flow,omitempty"`
}

type vlessJSONVnext struct {
	Address string          `json:"address"`
	Port    int             `json:"port"`
	Users   []vlessJSONUser `json:"users"`
}

type vlessJSONStreamSettings struct {
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

type vlessJSONSettings struct {
	Vnext []vlessJSONVnext `json:"vnext"`
}

func (c *vlessConfig) MarshalJSON() ([]byte, error) {
	user := vlessJSONUser{
		ID:         c.ID,
		Encryption: c.Encryption,
	}

	if c.Flow != "" {
		user.Flow = c.Flow
	}

	vnext := vlessJSONVnext{
		Address: c.Server,
		Port:    c.Port,
		Users:   []vlessJSONUser{user},
	}

	streamSettings := vlessJSONStreamSettings{
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
		if c.Type != "" {
			header["type"] = c.Type
		} else {
			header["type"] = "none"
		}
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
		if c.Host != "" {
			quicSettings["security"] = c.Host
		}
		if c.Path != "" {
			quicSettings["key"] = c.Path
		}
		header := make(map[string]interface{})
		if c.Type != "" {
			header["type"] = c.Type
		} else {
			header["type"] = "none"
		}
		quicSettings["header"] = header
		streamSettings.QUICSettings = quicSettings
	case "grpc":
		grpcSettings := make(map[string]interface{})
		if c.ServiceName != "" {
			grpcSettings["serviceName"] = c.ServiceName
		} else if c.Path != "" {
			grpcSettings["serviceName"] = c.Path
		}
		if c.Authority != "" {
			grpcSettings["authority"] = c.Authority
		}
		if c.Mode != "" {
			grpcSettings["multiMode"] = (c.Mode == "multi")
		}
		streamSettings.GRPCSettings = grpcSettings
	}

	switch c.Security {
	case "tls":
		streamSettings.Security = "tls"
		tlsSettings := make(map[string]interface{})
		if c.SNI != "" {
			tlsSettings["serverName"] = c.SNI
		} else if c.Host != "" {
			tlsSettings["serverName"] = c.Host
		}
		if c.ALPN != "" {
			alpnList := strings.Split(c.ALPN, ",")
			for i, alpn := range alpnList {
				alpnList[i] = strings.TrimSpace(alpn)
			}
			tlsSettings["alpn"] = alpnList
		}
		if len(tlsSettings) > 0 {
			streamSettings.TLSSettings = tlsSettings
		}
	case "reality":
		// Skip REALITY to avoid "empty password" errors - fall back to no security
		// REALITY requires complex configuration that's not available in URL format
		break
	}

	outboundConfig := map[string]interface{}{
		"protocol":       "vless",
		"settings":       vlessJSONSettings{Vnext: []vlessJSONVnext{vnext}},
		"streamSettings": streamSettings,
	}

	return json.Marshal(outboundConfig)
}

func parseVless(link string) (Config, error) {
	if !strings.HasPrefix(link, "vless://") {
		return nil, fmt.Errorf("invalid VLESS link format")
	}

	parsedURL, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("invalid VLESS link format: %v", err)
	}

	config := &vlessConfig{
		Raw: link,
	}

	if parsedURL.User != nil {
		config.ID = parsedURL.User.Username()
	}

	if config.ID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	host, portStr, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid VLESS link format: %v", err)
	}

	if err := validateAddress(host); err != nil {
		return nil, fmt.Errorf("invalid VLESS link format: %v", err)
	}

	config.Server = host
	config.Port, err = strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %v", err)
	}

	params := parsedURL.Query()

	config.Encryption = params.Get("encryption")
	if config.Encryption == "" {
		config.Encryption = "none"
	}

	config.Flow = params.Get("flow")
	config.Security = params.Get("security")
	config.SNI = params.Get("sni")
	config.ALPN = params.Get("alpn")

	config.Network = params.Get("type")
	if config.Network == "" {
		config.Network = "tcp"
	}

	config.Type = params.Get("headerType")
	config.Host = params.Get("host")
	config.Path = params.Get("path")
	config.Mode = params.Get("mode")
	config.Authority = params.Get("authority")
	config.ServiceName = params.Get("serviceName")

	if parsedURL.Fragment != "" {
		config.Remark, _ = url.QueryUnescape(parsedURL.Fragment)
	}

	if config.Server == "" {
		return nil, fmt.Errorf("server address is required")
	}
	if config.Port == 0 {
		return nil, fmt.Errorf("port is required")
	}

	if config.Encryption != "none" {
		return nil, fmt.Errorf("VLESS only supports 'none' encryption, got: %s", config.Encryption)
	}

	return config, nil
}
