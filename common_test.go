package kcp2k

import (
	"net"
	"testing"
)

// TestConnectionHash 测试连接哈希函数
func TestConnectionHash(t *testing.T) {
	// 创建不同的UDP地址
	endPointA := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7777}
	endPointB := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7777}
	endPointC := &net.UDPAddr{IP: net.ParseIP("127.9.0.1"), Port: 7777}
	endPointD := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7778}
	endPointE := &net.UDPAddr{IP: net.ParseIP("127.9.0.1"), Port: 7778}

	// 哈希值在不同的环境中会有所不同
	// 例如，Unity会有与.net core不同的哈希值
	// 因此我们不硬编码它们

	// 计算哈希值
	hashA := ConnectionHash(endPointA)
	hashB := ConnectionHash(endPointB)
	hashC := ConnectionHash(endPointC)
	hashD := ConnectionHash(endPointD)
	hashE := ConnectionHash(endPointE)

	// 相同的ip:port应该有相同的哈希值
	if hashA != hashB {
		t.Errorf("Same IP:Port should have same hash, got %d and %d", hashA, hashB)
	}

	// 不同的IP应该有不同的哈希值
	if hashC == hashA {
		t.Errorf("Different IP should have different hash, both got %d", hashA)
	}

	// 不同的端口应该有不同的哈希值
	if hashD == hashA {
		t.Errorf("Different port should have different hash, both got %d", hashA)
	}

	// 不同的ip:port应该有不同的哈希值
	if hashE == hashA {
		t.Errorf("Different IP:Port should have different hash, both got %d", hashA)
	}
}

// TestGenerateCookie 测试Cookie生成函数
func TestGenerateCookie(t *testing.T) {
	// Cookie不应该为0
	cookie1 := GenerateCookie()
	if cookie1 == 0 {
		t.Error("Generated cookie should not be 0")
	}

	// 两次生成的Cookie应该不同
	cookie2 := GenerateCookie()
	if cookie1 == cookie2 {
		t.Errorf("Two generated cookies should be different, both got %d", cookie1)
	}
}