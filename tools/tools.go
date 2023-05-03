/*
Copyright © 2022 - 2023 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tools

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/pkg/errors"
	ssh "golang.org/x/crypto/ssh"
)

type Host struct {
	XMLName xml.Name `xml:"host"`
	Mac     string   `xml:"mac,attr"`
	Name    string   `xml:"name,attr"`
	IP      string   `xml:"ip,attr"`
}

func GetHostNetConfig(regex, filePath string) (*Host, error) {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	r := regexp.MustCompile(regex)
	find := r.FindString(string(fileData))

	host := &Host{}
	if err = xml.Unmarshal([]byte(find), host); err != nil {
		return nil, err
	}

	return host, nil
}

func GetFileFromURL(url string, fileName string, skipVerify bool) error {
	if !skipVerify {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	data, err := http.Get(url)
	if err != nil {
		return err
	}
	defer data.Body.Close()

	// Create file
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}

	// Save data in file
	_, err = io.Copy(file, data.Body)
	return err
}

func GetFiles(dir string, pattern string) ([]string, error) {
	files, err := filepath.Glob(dir + "/" + pattern)
	if err != nil {
		return nil, err
	}

	if files != nil {
		return files, nil
	}

	return nil, err
}

// Sed code partially from https://forum.golangbridge.org/t/using-sed-in-golang/23526/16
func Sed(oldValue, newValue, filePath string) error {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Get file permissions
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	mode := info.Mode()

	// Regex is in the old value var
	regex := regexp.MustCompile(oldValue)
	fileString := string(fileData)
	fileString = regex.ReplaceAllString(fileString, newValue)
	fileData = []byte(fileString)

	err = os.WriteFile(filePath, fileData, mode)
	return err
}

type Client struct {
	Host     string
	Username string
	Password string
}

func (c *Client) clientConfig() *ssh.ClientConfig {
	sshConfig := &ssh.ClientConfig{
		User:            c.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(c.Password)},
		Timeout:         30 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return sshConfig
}

func (c *Client) connectToHost() (*ssh.Client, error) {
	// Define ssh connection
	sshConfig := c.clientConfig()

	// Connect to client
	sshClient, err := ssh.Dial("tcp", c.Host, sshConfig)
	if err != nil {
		return nil, err
	}

	return sshClient, nil
}

func (c *Client) RunSSH(cmd string) (string, error) {
	sshClient, err := c.connectToHost()
	if err != nil {
		// Failed to connect
		return "", err
	}
	defer sshClient.Close()

	// Open a client session
	session, err := sshClient.NewSession()
	if err != nil {
		// Failed to create session
		return "", err
	}
	defer session.Close()

	// Execute the command on the opened session
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Run(cmd); err != nil {
		// Failed to execute the command
		return stdout.String(), errors.Wrapf(err, stderr.String())
	}
	return stdout.String(), nil
}

func (c *Client) SendFile(src, dst, permission string) error {
	// Define ssh connection
	sshConfig := c.clientConfig()

	// Connect to client
	scpClient := scp.NewClientWithTimeout(c.Host, sshConfig, 10*time.Second)
	defer scpClient.Close()

	if err := scpClient.Connect(); err != nil {
		// Failed to connect
		return err
	}

	f, err := os.Open(src)
	if err != nil {
		// Failed to open
		return err
	}
	defer f.Close()

	if err := scpClient.CopyFile(context.Background(), f, dst, permission); err != nil {
		// Failed to copy
		return err
	}
	return nil
}

func HTTPShare(dir string, port int) error {
	// TODO: improve it to run in background!
	fs := http.FileServer(http.Dir(dir))
	http.Handle("/", fs)

	sPort := strconv.Itoa(port)
	err := http.ListenAndServe(":"+sPort, nil)
	return err
}
