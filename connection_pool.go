package kcp2k

import (
	"net"
)

// ConnectionPool 管理KcpServerConnection对象的复用
type ConnectionPool struct {
	pool       Pool[*KcpServerConnection]
	config     KcpConfig
	bufferPool *BufferPool
}

// NewConnectionPool 创建新的连接池
func NewConnectionPool(config KcpConfig, bufferPool *BufferPool) *ConnectionPool {
	return &ConnectionPool{
		pool: New(func() *KcpServerConnection {
			return &KcpServerConnection{
				config: config,
			}
		}),
		config:     config,
		bufferPool: bufferPool,
	}
}

// Get 从池中获取一个连接对象
func (cp *ConnectionPool) Get(
	onConnected func(*KcpServerConnection),
	onData func([]byte, KcpChannel),
	onDisconnected func(),
	onError func(ErrorCode, string),
	onRawSend func([]byte),
	cookie uint32,
	remote *net.UDPAddr,
) *KcpServerConnection {
	conn := cp.pool.Get()

	// 重置连接状态
	conn.onConnected = onConnected
	conn.onData = onData
	conn.onDisconnected = onDisconnected
	conn.onError = onError
	conn.onRawSend = onRawSend
	conn.config = cp.config
	conn.remoteAddr = remote

	// 重置或创建peer
	if conn.peer == nil {
		conn.peer = NewKcpPeerWithBufferPool(0, cookie, cp.config, conn, cp.bufferPool)
	} else {
		// 重置现有peer
		conn.peer.Reset(cp.config)
		conn.peer.Cookie = cookie
		conn.peer.Handler = conn
	}

	return conn
}

// Put 将连接对象归还到池中
func (cp *ConnectionPool) Put(conn *KcpServerConnection) {
	if conn == nil {
		return
	}

	// 清理连接状态
	conn.onConnected = nil
	conn.onData = nil
	conn.onDisconnected = nil
	conn.onError = nil
	conn.onRawSend = nil
	conn.remoteAddr = nil

	// 释放peer的缓冲区资源
	if conn.peer != nil {
		conn.peer.ReleaseBuffers()
	}

	// 不清理peer，保留用于复用
	// peer的Reset方法会在下次使用时调用

	cp.pool.Put(conn)
}
