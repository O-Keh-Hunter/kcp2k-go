# KCP2K 示例程序

本目录包含 KCP2K 协议的基础示例程序，帮助开发者理解和使用 KCP2K。

## 目录结构

### echo/
KCP 回显服务器和客户端示例
- **功能**: 演示基本的 KCP 连接、消息发送和接收
- **用法**: 
  ```bash
  # 启动服务器
  go run examples/echo/main.go server
  
  # 启动客户端
  go run examples/echo/main.go client
  ```

## 使用说明

1. **基础示例**: 从 `echo/` 开始，了解 KCP2K 的基本用法
2. **性能测试**: 压力测试程序已移至 `tests/` 目录
3. **自定义开发**: 参考示例代码开发自己的应用

## 构建说明

```bash
# 构建echo示例
go build -o examples/echo/echo ./examples/echo
```

## 其他测试

- **压力测试**: 请参考 `tests/` 目录下的压力测试程序
- **跨语言测试**: 请参考 `tests/` 目录下的跨语言兼容性测试