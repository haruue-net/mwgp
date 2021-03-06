package mwgp

import (
	"context"
	"fmt"
	"golang.zx2c4.com/wireguard/device"
	"log"
	"net"
	"time"
)

type ClientConfig struct {
	Server                    string         `json:"server"`
	Listen                    string         `json:"listen"`
	Timeout                   int            `json:"timeout"`
	DNS                       string         `json:"dns,omitempty"`
	ClientSourceValidateLevel int            `json:"csvl,omitempty"`
	ServerSourceValidateLevel int            `json:"ssvl,omitempty"`
	ClientPublicKey           NoisePublicKey `json:"client_pubkey"`
	ServerPublicKey           NoisePublicKey `json:"server_pubkey"`
}

type Client struct {
	wgitTable        *WireGuardIndexTranslationTable
	server           string
	cachedServerPeer ServerConfigPeer
}

func NewClientWithConfig(config *ClientConfig) (outClient *Client, err error) {
	if config.DNS != "" {
		net.DefaultResolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{}
				return d.DialContext(ctx, "udp", config.DNS)
			},
		}
	}

	client := Client{}
	client.wgitTable = NewWireGuardIndexTranslationTable()
	client.wgitTable.ClientListen, err = net.ResolveUDPAddr("udp", config.Listen)
	if err != nil {
		err = fmt.Errorf("invalid listen address %s: %w", config.Listen, err)
		return
	}
	client.wgitTable.Timeout = time.Duration(config.Timeout) * time.Second
	client.wgitTable.ExtractPeerFunc = client.generateServerPeer
	client.cachedServerPeer.serverPublicKey = config.ServerPublicKey
	client.cachedServerPeer.ClientPublicKey = &config.ClientPublicKey

	outClient = &client
	return
}

func (c *Client) generateServerPeer(msg *device.MessageInitiation) (fi *ServerConfigPeer, err error) {
	if c.cachedServerPeer.forwardToAddress == nil {
		err = fmt.Errorf("forward_to address is not resolved yet")
		return
	}
	fi = &c.cachedServerPeer
	return
}

func (c *Client) Start() (err error) {
	go func() {
		for {
			sa, rerr := net.ResolveUDPAddr("udp", c.server)
			if rerr != nil {
				log.Printf("[error] failed to resolve server addr %s: %s, retry in 10 seconds", c.server, rerr.Error())
				time.Sleep(10 * time.Second)
				continue
			}
			c.cachedServerPeer.forwardToAddress = sa
			c.wgitTable.UpdateAllServerDestinationChan <- sa
			time.Sleep(5 * time.Minute)
		}
	}()
	log.Printf("[info] listen on %s ...\n", c.wgitTable.ClientListen)
	err = c.wgitTable.Serve()
	return
}
