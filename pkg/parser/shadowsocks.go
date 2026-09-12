package parser

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"v2pingo/pkg/types"

	"github.com/sagernet/sing-box/option"
)

func parseShadowsocks(rawLink string, index int) (*types.ProxyConfig, error) {
	stripped := StripRemark(rawLink)
	body := strings.TrimPrefix(stripped, "ss://")
	body = strings.TrimPrefix(body, "SS://")
	if body == "" {
		return nil, fmt.Errorf("empty Shadowsocks link body")
	}

	var method, password, host string
	var port uint16
	var plugin, pluginOpts string

	if strings.Contains(body, "@") {
		// SIP002 format
		atIdx := strings.LastIndex(body, "@")
		userPart := body[:atIdx]
		hostPart := body[atIdx+1:]

		userDecoded := userPart
		if b, err := decodeBase64(userPart); err == nil && strings.Contains(string(b), ":") {
			userDecoded = string(b)
		}

		colonIdx := strings.Index(userDecoded, ":")
		if colonIdx == -1 {
			return nil, fmt.Errorf("invalid Shadowsocks userinfo format: %s", userDecoded)
		}
		method = userDecoded[:colonIdx]
		password = userDecoded[colonIdx+1:]

		hostPort := hostPart
		if qIdx := strings.Index(hostPart, "?"); qIdx != -1 {
			hostPort = hostPart[:qIdx]
			queryStr := hostPart[qIdx+1:]
			q, _ := url.ParseQuery(queryStr)
			pluginVal := q.Get("plugin")
			if pluginVal != "" {
				if semiIdx := strings.Index(pluginVal, ";"); semiIdx != -1 {
					plugin = pluginVal[:semiIdx]
					pluginOpts = pluginVal[semiIdx+1:]
				} else {
					plugin = pluginVal
				}
			}
		}

		h, pStr, err := net.SplitHostPort(hostPort)
		if err != nil {
			return nil, fmt.Errorf("invalid host:port in Shadowsocks URI: %w", err)
		}
		host = h
		pNum, err := strconv.ParseUint(pStr, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q in Shadowsocks URI: %w", pStr, err)
		}
		port = uint16(pNum)
	} else {
		// Legacy format: base64(method:password@host:port)
		decodedBytes, err := decodeBase64(body)
		if err != nil {
			return nil, fmt.Errorf("failed to decode legacy Shadowsocks URI: %w", err)
		}
		decodedStr := string(decodedBytes)
		atIdx := strings.LastIndex(decodedStr, "@")
		if atIdx == -1 {
			return nil, fmt.Errorf("missing @ in legacy Shadowsocks URI payload: %s", decodedStr)
		}

		userPart := decodedStr[:atIdx]
		hostPort := decodedStr[atIdx+1:]

		colonIdx := strings.Index(userPart, ":")
		if colonIdx == -1 {
			return nil, fmt.Errorf("invalid method:password in legacy Shadowsocks URI: %s", userPart)
		}
		method = userPart[:colonIdx]
		password = userPart[colonIdx+1:]

		h, pStr, err := net.SplitHostPort(hostPort)
		if err != nil {
			return nil, fmt.Errorf("invalid host:port in legacy Shadowsocks URI: %w", err)
		}
		host = h
		pNum, err := strconv.ParseUint(pStr, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q in legacy Shadowsocks URI: %w", pStr, err)
		}
		port = uint16(pNum)
	}

	if method == "" || password == "" || host == "" || port == 0 {
		return nil, fmt.Errorf("incomplete Shadowsocks configuration")
	}

	ssOpt := option.ShadowsocksOutboundOptions{
		ServerOptions: option.ServerOptions{
			Server:     host,
			ServerPort: port,
		},
		Method:        method,
		Password:      password,
		Plugin:        plugin,
		PluginOptions: pluginOpts,
	}

	return &types.ProxyConfig{
		Index:    index,
		Protocol: "shadowsocks",
		Host:     host,
		Port:     port,
		RawLink:  rawLink,
		Options: option.Outbound{
			Type:    "shadowsocks",
			Tag:     "proxy",
			Options: ssOpt,
		},
	}, nil
}
