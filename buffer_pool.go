package kcp2k

// BufferPool 管理大型缓冲区的复用
type BufferPool struct {
	messageBufferPool Pool[[]byte]
	sendBufferPool    Pool[[]byte]
	rawBufferPool     Pool[[]byte]
}

// NewBufferPool 创建新的缓冲区池
func NewBufferPool(config KcpConfig) *BufferPool {
	reliableMax := ReliableMaxMessageSize(config.Mtu, config.ReceiveWindowSize)
	if reliableMax < 0 {
		reliableMax = 0
	}

	return &BufferPool{
		messageBufferPool: New(func() []byte {
			return make([]byte, 1+reliableMax)
		}),
		sendBufferPool: New(func() []byte {
			return make([]byte, 1+reliableMax)
		}),
		rawBufferPool: New(func() []byte {
			return make([]byte, config.Mtu)
		}),
	}
}

// GetMessageBuffer 获取消息缓冲区
func (bp *BufferPool) GetMessageBuffer() []byte {
	return bp.messageBufferPool.Get()
}

// PutMessageBuffer 归还消息缓冲区
func (bp *BufferPool) PutMessageBuffer(buf []byte) {
	bp.messageBufferPool.Put(buf)
}

// GetSendBuffer 获取发送缓冲区
func (bp *BufferPool) GetSendBuffer() []byte {
	return bp.sendBufferPool.Get()
}

// PutSendBuffer 归还发送缓冲区
func (bp *BufferPool) PutSendBuffer(buf []byte) {
	bp.sendBufferPool.Put(buf)
}

// GetRawBuffer 获取原始缓冲区
func (bp *BufferPool) GetRawBuffer() []byte {
	return bp.rawBufferPool.Get()
}

// PutRawBuffer 归还原始缓冲区
func (bp *BufferPool) PutRawBuffer(buf []byte) {
	bp.rawBufferPool.Put(buf)
}
