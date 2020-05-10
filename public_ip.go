package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

type CurrentIP struct {
	ip  string
	mux sync.Mutex
}

func (c *CurrentIP) Get() string {
	c.mux.Lock()
	defer c.mux.Unlock()
	return c.ip
}

func (c *CurrentIP) Set(ip string) {
	c.mux.Lock()
	c.ip = ip
	c.mux.Unlock()
}

func getPublicIP() (ip string, err error) {
	resp, err := http.Get("http://api.ipify.org")
	if err != nil {
		return "", err
	}
	publicIPRaw, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return "", err
	}

	return string(publicIPRaw), nil
}

func watchPublicIP(currentIP *CurrentIP) {
	ticker := time.NewTicker(30 * time.Second)
	for {
		publicIP, err := getPublicIP()
		if err != nil {
			println("Could not retrieve IP. Retry on next check...")
		}
		if currentIP.Get() != publicIP {
			currentIP.Set(publicIP)
		}
		println(currentIP.Get())
		<-ticker.C
	}
}

func waitForPublicIP(currentIP *CurrentIP) {
	ticker := time.NewTicker(time.Second)
	for {
		ip := currentIP.Get()
		if ip != "" {
			fmt.Println("Public IP: " + ip)
			break
		} else {
			fmt.Println("Waiting to get public IP...")
		}
		<-ticker.C
	}
}
