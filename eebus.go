package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/andig/evcc/hems/eebus/ship"
	"github.com/andig/evcc/hems/semp"

	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"

	"github.com/andig/evcc/hems/eebus"
	"github.com/gorilla/websocket"
	"github.com/grandcat/zeroconf"
)

const (
	zeroconfType   = "_ship._tcp"
	zeroconfDomain = "local."
)

func discoverDNS(results <-chan *zeroconf.ServiceEntry) {
	for entry := range results {
		// log.Printf("%+v", entry)
		ss, err := eebus.NewFromDNSEntry(entry)
		if err == nil {
			err = ss.Connect()
			log.Printf("%s: %+v", entry.HostName, ss)
		}

		if err == nil {
			err = ss.Close()
		}

		if err != nil {
			log.Println(err)
		}
	}
}

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func pemBlockForKey(priv interface{}) *pem.Block {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to marshal ECDSA private key: %v", err)
			os.Exit(2)
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}
	default:
		return nil
	}
}

func createCertificate(isCA bool, hosts ...string) tls.Certificate {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 180),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}
	if isCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(priv), priv)
	if err != nil {
		log.Fatalf("Failed to create certificate: %s", err)
	}

	out := &bytes.Buffer{}
	pem.Encode(out, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	fmt.Println(out.String())

	out.Reset()
	pem.Encode(out, pemBlockForKey(priv))
	fmt.Println(out.String())

	// tls.LoadX509KeyPair(certFile, keyFile)
	tlsCert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}

	return tlsCert
}

func SelfSigned(uri string) (*websocket.Conn, error) {
	ips := semp.LocalIPs()
	ip := ips[0]
	log.Println("using ip: " + ip.String())

	certPool, err := x509.SystemCertPool()
	if err != nil {
		log.Fatal(err)
	}

	tlsClientCert := createCertificate(false, ip.String())
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 5 * time.Second,
		TLSClientConfig: &tls.Config{
			RootCAs:            certPool,
			Certificates:       []tls.Certificate{tlsClientCert},
			InsecureSkipVerify: true,
		},
		Subprotocols: []string{ship.SubProtocol},
	}

	log.Println("using uri: " + uri)

	conn, resp, err := dialer.Dial(uri, http.Header{})
	fmt.Println(resp)

	return conn, err
}

const serverConf = 42424

func main() {
	// Discover all services on the network (e.g. _workstation._tcp)
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Fatalln("Failed to initialize resolver:", err.Error())
	}

	// created signed connections
	eebus.Connector = SelfSigned

	cert := createCertificate(true, "evcc")
	// ski := cert.SubjectKeyId
	_ = cert
	ski := "cert.SubjectKeyId"
	fmt.Printf("ski: %0x\n", ski)

	server, err := zeroconf.Register("evcc", zeroconfType, zeroconfDomain, serverConf, []string{"ski=" + ski, "register=true"}, nil)
	if err != nil {
		panic(err)
	}
	defer server.Shutdown()

	entries := make(chan *zeroconf.ServiceEntry)
	go discoverDNS(entries)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	if err = resolver.Browse(ctx, zeroconfType, zeroconfDomain, entries); err != nil {
		log.Fatalln("Failed to browse:", err.Error())
	}

	<-ctx.Done()
}
