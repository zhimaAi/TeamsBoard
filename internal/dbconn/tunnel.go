// Package dbconn provides database connection management and supports connection through SSH tunnel (local port forwarding)
// A database that cannot be accessed directly by the client but is reachable by the intranet of the springboard machine.
package dbconn

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/ssh"
)

// ConnectionTestTimeout Timeout for SSH and database connection.
const ConnectionTestTimeout = 10 * time.Second

// SSHConfig describes the SSH tunnel springboard configuration.
type SSHConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	KeyPath  string
}

// DBConfig describes the target database configuration. Host/Port is the address reachable by the internal network of the springboard machine.
type DBConfig struct {
	Type         string
	Host         string
	Port         int
	DatabaseName string
	Username     string
	Password     string
}

// Tunnel holds the SSH client and local listening port, and calls Close to release the resources after use.
type Tunnel struct {
	listener  net.Listener
	sshClient *ssh.Client
	localPort int
}

// Open opens a connection to the target database.
// If ssh is nil or Host is empty, connect directly; otherwise, establish a local port forwarding tunnel,
// DSN is rewritten to point to 127.0.0.1: the local port.
// The returned cleanup must be called after *sql.DB is no longer used to close the SSH connection and local listening.
func Open(ctx context.Context, db DBConfig, ssh *SSHConfig) (*sql.DB, func(), error) {
	driverName, dsn, err := driverAndDSN(db, db.Host, db.Port)
	if err != nil {
		return nil, nil, err
	}

	var tunnel *Tunnel
	if ssh != nil && ssh.Host != "" {
		tunnel, err = openTunnel(ctx, *ssh, db.Host, db.Port)
		if err != nil {
			return nil, nil, fmt.Errorf("建立 SSH 隧道失败: %w", err)
		}
		driverName, dsn, err = driverAndDSN(db, "127.0.0.1", tunnel.localPort)
		if err != nil {
			tunnel.Close()
			return nil, nil, err
		}
	}

	dbHandle, err := sql.Open(driverName, dsn)
	if err != nil {
		if tunnel != nil {
			tunnel.Close()
		}
		return nil, nil, fmt.Errorf("打开数据库连接失败: %w", err)
	}

	cleanup := func() {
		_ = dbHandle.Close()
		if tunnel != nil {
			_ = tunnel.Close()
		}
	}
	return dbHandle, cleanup, nil
}

// ResolveSSHConfig resolves the SSH configuration from the gt_ssh_profiles table based on ssh_profile_id.
// db is the application database handle; secretLookup is used to retrieve the password from secret_ref.
// Return nil when id<=0 (indicating direct connection, no tunnel).
func ResolveSSHConfig(ctx context.Context, db *sql.DB, secretLookup func(ref string) string, sshProfileID int64) (*SSHConfig, error) {
	if sshProfileID <= 0 {
		return nil, nil
	}
	var host string
	var port int
	var username, secretRef, keyPath string
	err := db.QueryRowContext(ctx,
		`SELECT host, port, username, secret_ref, key_path FROM gt_ssh_profiles WHERE id = ?`, sshProfileID).
		Scan(&host, &port, &username, &secretRef, &keyPath)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("关联的 SSH 配置不存在")
	}
	if err != nil {
		return nil, err
	}
	return &SSHConfig{
		Host:     host,
		Port:     port,
		Username: username,
		Password: secretLookup(secretRef),
		KeyPath:  keyPath,
	}, nil
}

// driverAndDSN generates the driver name and connection string according to the database type.
//
// Note: fmt.Sprintf cannot be used to manually splice DSN. When characters such as @ : / # spaces and single quotes appear in the password,
// Manual splicing will cause the driver to truncate the password or fail to parse it. Here we use the driver’s official structured constructor instead:
// MySQL uses mysql.Config.FormatDSN(), PostgreSQL uses URL form and is escaped by url.UserPassword.
func driverAndDSN(db DBConfig, host string, port int) (string, string, error) {
	switch strings.ToLower(db.Type) {
	case "mysql":
		if port == 0 {
			port = 3306
		}
		cfg := mysql.NewConfig()
		cfg.Net = "tcp"
		cfg.Addr = net.JoinHostPort(host, strconv.Itoa(port))
		cfg.User = db.Username
		cfg.Passwd = db.Password
		cfg.DBName = db.DatabaseName
		cfg.Timeout = ConnectionTestTimeout
		cfg.ReadTimeout = ConnectionTestTimeout
		return "mysql", cfg.FormatDSN(), nil
	case "postgresql", "postgres", "pgsql":
		if port == 0 {
			port = 5432
		}
		u := &url.URL{
			Scheme: "postgres",
			Host:   net.JoinHostPort(host, strconv.Itoa(port)),
			Path:   "/" + db.DatabaseName,
		}
		if db.Username != "" {
			u.User = url.UserPassword(db.Username, db.Password)
		}
		query := url.Values{}
		query.Set("sslmode", "disable")
		query.Set("connect_timeout", strconv.Itoa(int(ConnectionTestTimeout.Seconds())))
		u.RawQuery = query.Encode()
		return "postgres", u.String(), nil
	default:
		return "", "", fmt.Errorf("不支持的数据库类型: %s", db.Type)
	}
}

// openTunnel establishes an SSH connection and listens locally, forwarding the connection to the target host:port.
func openTunnel(ctx context.Context, sshCfg SSHConfig, targetHost string, targetPort int) (*Tunnel, error) {
	authMethods, err := buildSSHAuthMethods(sshCfg)
	if err != nil {
		return nil, err
	}
	port := sshCfg.Port
	if port == 0 {
		port = 22
	}
	clientConfig := &ssh.ClientConfig{
		User:            sshCfg.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         ConnectionTestTimeout,
	}
	address := net.JoinHostPort(sshCfg.Host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: ConnectionTestTimeout}
	netConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("无法连接 SSH 主机: %w", err)
	}
	conn, channels, requests, err := ssh.NewClientConn(netConn, address, clientConfig)
	if err != nil {
		netConn.Close()
		return nil, fmt.Errorf("SSH 认证或握手失败: %w", err)
	}
	sshClient := ssh.NewClient(conn, channels, requests)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("本地监听失败: %w", err)
	}
	localPort := listener.Addr().(*net.TCPAddr).Port

	targetAddr := net.JoinHostPort(targetHost, strconv.Itoa(targetPort))
	go func() {
		for {
			localConn, err := listener.Accept()
			if err != nil {
				return
			}
			go forward(localConn, sshClient, targetAddr)
		}
	}()

	return &Tunnel{listener: listener, sshClient: sshClient, localPort: localPort}, nil
}

// forward forwards data in both directions between the local connection and the target address.
func forward(localConn net.Conn, sshClient *ssh.Client, targetAddr string) {
	defer localConn.Close()
	remoteConn, err := sshClient.Dial("tcp", targetAddr)
	if err != nil {
		return
	}
	defer remoteConn.Close()
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(remoteConn, localConn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(localConn, remoteConn)
		done <- struct{}{}
	}()
	<-done
}

// Close releases tunnel resources (closes local listening and SSH client).
func (t *Tunnel) Close() error {
	if t.listener != nil {
		_ = t.listener.Close()
	}
	if t.sshClient != nil {
		_ = t.sshClient.Close()
	}
	return nil
}

// buildSSHAuthMethods generates SSH authentication methods based on password/private key.
func buildSSHAuthMethods(cfg SSHConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if cfg.Password != "" {
		methods = append(methods, ssh.Password(cfg.Password))
	}
	if cfg.KeyPath != "" {
		keyData, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("读取私钥失败: %w", err)
		}
		var signer ssh.Signer
		if cfg.Password != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(cfg.Password))
		} else {
			signer, err = ssh.ParsePrivateKey(keyData)
		}
		if err != nil {
			return nil, fmt.Errorf("解析私钥失败: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("未配置密码或私钥，无法认证")
	}
	return methods, nil
}
