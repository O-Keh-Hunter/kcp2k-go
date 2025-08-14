# KCP2K 工具集

本目录包含 KCP2K 项目的各种工具和脚本。

## 目录结构

### scripts/
项目相关的脚本工具

#### scripts/testing/
测试相关脚本
- `run_all_tests.sh` - 运行所有跨语言兼容性测试
- `run_csharp_server_go_client.sh` - C# 服务端 + Go 客户端测试
- `run_go_server_csharp_client.sh` - Go 服务端 + C# 客户端测试

#### scripts/performance/
性能测试脚本
- `test_full.sh` - 完整规模压力测试 (1000服务器 × 10客户端)
- `test_small.sh` - 小规模压力测试 (10服务器 × 5客户端)

**注意**: 压力测试的可执行程序（stress_server、stress_client、stress_test）已从 `examples/` 目录移动到 `tests/` 目录，以便更好地组织测试相关代码。

## 使用说明

### 运行跨语言测试
```bash
# 运行所有测试
./tools/scripts/testing/run_all_tests.sh

# 运行特定场景测试
./tools/scripts/testing/run_csharp_server_go_client.sh
./tools/scripts/testing/run_go_server_csharp_client.sh
```

### 运行性能测试
```bash
# 小规模测试 (推荐先运行)
./tools/scripts/performance/test_small.sh

# 完整规模测试 (需要大量系统资源)
./tools/scripts/performance/test_full.sh
```

## 注意事项

1. **权限**: 确保脚本有执行权限 (`chmod +x script_name.sh`)
2. **依赖**: 性能测试需要先构建相关的示例程序
3. **资源**: 完整规模测试需要大量 CPU 和内存资源
4. **平台**: 脚本主要在 Unix-like 系统上测试 (Linux/macOS)