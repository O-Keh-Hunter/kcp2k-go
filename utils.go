package kcp2k

import (
	"encoding/binary"
)

// 二进制编码工具函数，对应 C# 版的 Utils.Encode32U/Decode32U

// Encode32U 将 uint32 编码到字节切片的指定位置
func Encode32U(data []byte, offset int, value uint32) {
	if offset+4 <= len(data) {
		binary.LittleEndian.PutUint32(data[offset:], value)
	}
}

// Decode32U 从字节切片的指定位置解码 uint32
func Decode32U(data []byte, offset int) (uint32, bool) {
	if offset+4 <= len(data) {
		value := binary.LittleEndian.Uint32(data[offset:])
		return value, true
	}
	return 0, false
}

// 队列计数工具函数
func GetSendQueueCount(kcp interface{}) int {
	if k, ok := kcp.(interface{ NsndQueue() int }); ok {
		return k.NsndQueue()
	}
	return 0
}

func GetReceiveQueueCount(kcp interface{}) int {
	if k, ok := kcp.(interface{ NrcvQueue() int }); ok {
		return k.NrcvQueue()
	}
	return 0
}

func GetSendBufferCount(kcp interface{}) int {
	if k, ok := kcp.(interface{ NsndBuf() int }); ok {
		return k.NsndBuf()
	}
	return 0
}

func GetReceiveBufferCount(kcp interface{}) int {
	if k, ok := kcp.(interface{ NrcvBuf() int }); ok {
		return k.NrcvBuf()
	}
	return 0
}
