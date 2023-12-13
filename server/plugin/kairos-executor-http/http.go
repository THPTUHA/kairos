package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/THPTUHA/kairos/pkg/circbuf"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
)

const (
	timeout    = 30
	maxBufSize = 256000
)

type HTTP struct {
}

func (s *HTTP) Execute(args *proto.ExecuteRequest, cb plugin.StatusHelper) (*proto.ExecuteResponse, error) {
	out, err := s.ExecuteImpl(args)
	resp := &proto.ExecuteResponse{Output: out}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func (s *HTTP) ExecuteImpl(args *proto.ExecuteRequest) ([]byte, error) {
	output := circbuf.NewBuffer(maxBufSize)
	var debug bool
	if args.Config["debug"] != "" {
		debug = true
		output.Write([]byte(fmt.Sprintf("Config: %#v\n", args.Config)))
	}

	if args.Config["url"] == "" {
		return output.Bytes(), errors.New("url is empty")
	}

	if args.Config["method"] == "" {
		return output.Bytes(), errors.New("method is empty")
	}

	req, err := http.NewRequest(args.Config["method"], args.Config["url"], bytes.NewBuffer([]byte(args.Config["body"])))
	if err != nil {
		return output.Bytes(), err
	}
	req.Close = true

	var headers []string
	if args.Config["headers"] != "" {
		if err := json.Unmarshal([]byte(args.Config["headers"]), &headers); err != nil {
			output.Write([]byte("Error: parsing headers failed\n"))
		}
	}

	for _, h := range headers {
		if h != "" {
			kv := strings.Split(h, ":")
			req.Header.Set(kv[0], strings.TrimSpace(kv[1]))
		}
	}
	if debug {
		log.Printf("request  %#v\n\n", req)
	}

	client, warns := createClient(args.Config)
	for _, warn := range warns {
		output.Write([]byte(fmt.Sprintf("Warning: %s.\n", warn.Error())))
	}

	resp, err := client.Do(req)
	if err != nil {
		return output.Bytes(), err
	}

	defer resp.Body.Close()
	out, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return output.Bytes(), err
	}

	if debug {
		log.Printf("response  %#v\n\n", resp)
		log.Printf("response body  %#v\n\n", string(out))
	}

	_, err = output.Write(out)
	if err != nil {
		return output.Bytes(), err
	}

	if args.Config["expectCode"] != "" && !strings.Contains(args.Config["expectCode"]+",", fmt.Sprintf("%d,", resp.StatusCode)) {
		return output.Bytes(), errors.New("received response code does not match the expected code")
	}

	if args.Config["expectBody"] != "" {
		if m, _ := regexp.MatchString(args.Config["expectBody"], string(out)); !m {
			return output.Bytes(), errors.New("received response body did not match the expected body")
		}
	}

	if output.TotalWritten() > output.Size() {
		log.Printf("'%s %s': generated %d bytes of output, truncated to %d",
			args.Config["method"], args.Config["url"],
			output.TotalWritten(), output.Size())
	}

	return output.Bytes(), nil
}

func createClient(config map[string]string) (http.Client, []error) {
	var errs []error

	_timeout, err := atoiOrDefault(config["timeout"], timeout)
	if config["timeout"] != "" && err != nil {
		errs = append(errs, fmt.Errorf("invalid timeout value: %s", err.Error()))
	}

	tlsconf := &tls.Config{}
	tlsconf.InsecureSkipVerify, err = strconv.ParseBool(config["tlsNoVerifyPeer"])
	if config["tlsNoVerifyPeer"] != "" && err != nil {
		errs = append(errs, fmt.Errorf("not disabling certificate validation: %s", err.Error()))
	}

	if config["tlsCertificateFile"] != "" {
		cert, err := tls.LoadX509KeyPair(config["tlsCertificateFile"], config["tlsCertificateKeyFile"])
		if err == nil {
			tlsconf.Certificates = append(tlsconf.Certificates, cert)
		} else {
			errs = append(errs, fmt.Errorf("not using client certificate: %s", err.Error()))
		}
	}

	if config["tlsRootCAsFile"] != "" {
		tlsconf.RootCAs, err = loadCertPool(config["tlsRootCAsFile"])
		if err != nil {
			errs = append(errs, fmt.Errorf("using system root CAs instead of configured CAs: %s", err.Error()))
		}
	}

	return http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsconf},
		Timeout:   time.Duration(_timeout) * time.Second,
	}, errs
}

func loadCertPool(filename string) (*x509.CertPool, error) {
	certsFile, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certsFile) {
		return nil, fmt.Errorf("no certificates in file")
	}

	return roots, nil
}

func atoiOrDefault(s string, _default int) (int, error) {
	i, err := strconv.Atoi(s)
	if err == nil {
		return i, nil
	}
	return _default, fmt.Errorf("\"%s\" not understood (%s), using default value of %d", s, err, _default)
}
