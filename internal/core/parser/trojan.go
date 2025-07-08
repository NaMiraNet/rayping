package parser

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type trojanConfig struct {
	Raw           string `json:"-"`
	Server        string `json:"server"`
	Port          int    `json:"port"`
	Password      string `json:"password"`
	SNI           string `json:"sni,omitempty"`
	ALPN          string `json:"alpn,omitempty"`
	Network       string `json:"network"`
	Type          string `json:"type,omitempty"`
	Host          string `json:"host,omitempty"`
	Path          string `json:"path,omitempty"`
	Mode          string `json:"mode,omitempty"`
	Authority     string `json:"authority,omitempty"`
	ServiceName   string `json:"serviceName,omitempty"`
	Security      string `json:"security,omitempty"`
	AllowInsecure bool   `json:"allowInsecure,omitempty"`
	Remark        string `json:"remark,omitempty"`
}

type trojanJSONServer struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Password string `json:"password"`
}

type trojanJSONStreamSettings struct {
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

type trojanJSONSettings struct {
	Servers []trojanJSONServer `json:"servers"`
}

func (c *trojanConfig) MarshalJSON() ([]byte, error) {
	server := trojanJSONServer{
		Address:  c.Server,
		Port:     c.Port,
		Password: c.Password,
	}

	streamSettings := trojanJSONStreamSettings{
		Network: c.Network,
	}

	// Only set TLS security for Trojan (no REALITY support to avoid empty password error)
	if c.Security == "" || c.Security == "tls" {
		streamSettings.Security = "tls"
	} else if c.Security == "reality" {
		// Skip REALITY for now to avoid configuration errors
		streamSettings.Security = "tls"
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

	if streamSettings.Security == "tls" {
		tlsSettings := make(map[string]interface{})
		if c.SNI != "" {
			tlsSettings["serverName"] = c.SNI
		} else if c.Host != "" {
			tlsSettings["serverName"] = c.Host
		} else {
			tlsSettings["serverName"] = c.Server
		}

		if c.ALPN != "" {
			alpnList := strings.Split(c.ALPN, ",")
			for i, alpn := range alpnList {
				alpnList[i] = strings.TrimSpace(alpn)
			}
			tlsSettings["alpn"] = alpnList
		}

		if c.AllowInsecure {
			tlsSettings["allowInsecure"] = true
		}

		streamSettings.TLSSettings = tlsSettings
	}

	outboundConfig := map[string]interface{}{
		"protocol":       "trojan",
		"settings":       trojanJSONSettings{Servers: []trojanJSONServer{server}},
		"streamSettings": streamSettings,
	}

	return json.Marshal(outboundConfig)
}

func parseTrojan(link string) (Config, error) {
	if !strings.HasPrefix(link, "trojan://") {
		return nil, fmt.Errorf("invalid Trojan link format")
	}

	parsedURL, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("invalid Trojan link format: %v", err)
	}

	config := &trojanConfig{
		Raw: link,
	}

	// Get password from URL user info
	if parsedURL.User != nil {
		config.Password = parsedURL.User.Username()
	}

	if config.Password == "" {
		return nil, fmt.Errorf("password is required")
	}

	host, portStr, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid Trojan link format: %v", err)
	}

	config.Server = host
	if err := validateAddress(config.Server); err != nil {
		return nil, fmt.Errorf("invalid VMess link format: %v", err)
	}

	config.Port, err = strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %v", err)
	}
	params := parsedURL.Query()

	config.SNI = params.Get("sni")
	if config.SNI == "" {
		config.SNI = params.Get("peer")
	}

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

	config.Security = params.Get("security")
	if config.Security == "" {
		config.Security = "tls"
	}

	if params.Get("allowInsecure") == "1" || params.Get("allowInsecure") == "true" {
		config.AllowInsecure = true
	}
	if params.Get("skipCertVerify") == "1" || params.Get("skipCertVerify") == "true" {
		config.AllowInsecure = true
	}

	if parsedURL.Fragment != "" {
		config.Remark, _ = url.QueryUnescape(parsedURL.Fragment)
	}

	if config.Server == "" {
		return nil, fmt.Errorf("server address is required")
	}
	if config.Port == 0 {
		return nil, fmt.Errorf("port is required")
	}

	return config, nil
}
