package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type ssConfig struct {
	Raw      string `json:"-"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Method   string `json:"method"`
	Password string `json:"password"`
	Remark   string `json:"remark"`
}

type ssJSONServer struct {
	Address  string `json:"address"`
	Method   string `json:"method"`
	OTA      bool   `json:"ota"`
	Password string `json:"password"`
	Port     int    `json:"port"`
}

type ssJSONSettings struct {
	Servers []ssJSONServer `json:"servers"`
}

func (c *ssConfig) MarshalJSON() ([]byte, error) {
	server := ssJSONServer{
		Address:  c.Server,
		Method:   c.Method,
		Password: c.Password,
		Port:     c.Port,
	}
	return json.Marshal(basicOutboundConfig{
		Protocol: "shadowsocks",
		Settings: ssJSONSettings{Servers: []ssJSONServer{server}},
	})
}

func parseSS(link string) (Config, error) {
	if !strings.HasPrefix(link, "ss://") {
		return nil, fmt.Errorf("invalid ShadowSocks link format")
	}

	// Remove ss:// prefix
	link = strings.TrimPrefix(link, "ss://")

	config := &ssConfig{Raw: link}

	// Handle remark (fragment)
	parts := strings.Split(link, "#")
	if len(parts) == 2 {
		config.Remark, _ = url.QueryUnescape(parts[1])
		link = parts[0]
	}

	// Split by @ to separate auth and server info
	atIndex := strings.LastIndex(link, "@")
	if atIndex == -1 {
		return nil, fmt.Errorf("invalid ShadowSocks link format")
	}

	authPart := link[:atIndex]
	serverPart := link[atIndex+1:]

	// Decode auth part (method:password)
	authData, err := base64.StdEncoding.DecodeString(authPart)
	if err != nil {
		// Try URL-safe base64
		authData, err = base64.URLEncoding.DecodeString(authPart)
		if err != nil {
			// Try raw standard encoding
			authData, err = base64.RawStdEncoding.DecodeString(authPart)
			if err != nil {
				// Try raw URL encoding
				authData, err = base64.RawURLEncoding.DecodeString(authPart)
				if err != nil {
					return nil, fmt.Errorf("invalid ShadowSocks link format: failed to decode auth")
				}
			}
		}
	}

	auth := strings.Split(string(authData), ":")
	if len(auth) != 2 {
		return nil, fmt.Errorf("invalid ShadowSocks link format: invalid auth format")
	}
	config.Method = auth[0]
	config.Password = auth[1]

	// Parse server and port
	host, portStr, err := net.SplitHostPort(serverPart)
	if err != nil {
		return nil, fmt.Errorf("invalid ShadowSocks link format: invalid server format")
	}

	config.Server = host
	if err := validateAddress(config.Server); err != nil {
		return nil, fmt.Errorf("invalid ShadowSocks link format: %v", err)
	}

	config.Port, err = strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid ShadowSocks link format: invalid port")
	}

	return config, nil
}
