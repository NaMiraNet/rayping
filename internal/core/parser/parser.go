package parser

import (
	"fmt"
	"strings"
)

type Config interface {
	MarshalJSON() ([]byte, error)
}

var (
	ErrInvalidConfig    = fmt.Errorf("invalid config")
	ErrUnsupportedType  = fmt.Errorf("unsupported config type")
	ErrInvalidFormat    = fmt.Errorf("invalid config format")
	ErrLocalhostBlocked = fmt.Errorf("localhost addresses are not allowed")
)

type basicOutboundConfig struct {
	Protocol string `json:"protocol"`
	Settings any    `json:"settings"`
}

type ConfigParser func(string) (Config, error)

type Parser struct {
	parsers map[string]ConfigParser
}

func NewParser() *Parser {
	return &Parser{
		parsers: map[string]ConfigParser{
			"ss":     parseSS,
			"vless":  parseVless,
			"vmess":  parseVMess,
			"trojan": parseTrojan,
		},
	}
}

func (p *Parser) Parse(config string) (Config, error) {
	if config == "" {
		return nil, ErrInvalidFormat
	}

	parts := strings.SplitN(config, "://", 2)
	if len(parts) != 2 {
		return nil, ErrInvalidFormat
	}

	protocol := strings.ToLower(parts[0])
	parser, exists := p.parsers[protocol]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedType, protocol)
	}

	return parser(config)
}

func (p *Parser) AddParser(protocol string, parser ConfigParser) {
	p.parsers[protocol] = parser
}

func (p *Parser) RemoveParser(protocol string) {
	delete(p.parsers, protocol)
}

func (p *Parser) SupportedProtocols() []string {
	protocols := make([]string, 0, len(p.parsers))
	for protocol := range p.parsers {
		protocols = append(protocols, protocol)
	}
	return protocols
}
