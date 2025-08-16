# LockStep 最小玩家数功能测试

本目录包含了用于测试和演示 LockStep 系统最小玩家数功能的脚本。

## 功能说明

最小玩家数功能确保游戏只在达到配置的最小玩家数时才开始运行：

- **默认配置**：最小玩家数 = 2，最大玩家数 = 8
- **等待状态**：当玩家数少于最小值时，游戏保持等待状态 (Status=0)
- **开始游戏**：当玩家数达到最小值时，游戏自动开始 (Status=2)
- **继续运行**：更多玩家加入时，游戏继续运行

## 测试脚本

### 1. 自动化测试脚本

```bash
./test_min_players.sh
```

**功能**：
- 自动启动服务器和多个客户端
- 验证各个阶段的游戏状态
- 生成详细的测试报告
- 自动清理测试进程

**测试流程**：
1. 启动服务器，验证配置
2. 连接第1个客户端，验证游戏等待状态
3. 连接第2个客户端，验证游戏开始
4. 连接第3个客户端，验证游戏继续运行
5. 生成测试报告并清理

### 2. 交互式演示脚本

```bash
./demo_min_players.sh
```

**功能**：
- 分步骤演示最小玩家数功能
- 实时显示服务器状态
- 用户可以观察每个阶段的变化
- 按 Enter 键结束演示

### 3. 第三个玩家同步测试脚本

```bash
./test_third_player_sync.sh
```

**功能**：
- 测试游戏运行中新玩家加入的帧数据同步
- 验证新玩家能否正确接收历史帧数据
- 确保游戏状态同步的完整性

**演示流程**：
1. 显示服务器配置信息
2. 连接第1个玩家，展示等待状态
3. 连接第2个玩家，展示游戏开始
4. 显示实时游戏状态
5. 等待用户确认后清理

## 手动测试

如果你想手动测试，可以按以下步骤：

### 1. 编译程序

```bash
go build -o lockstep-server main.go
```

### 2. 启动服务器

```bash
./lockstep-server server 8888
```

服务器将显示：
```
Room config: MinPlayers=2, MaxPlayers=8, FrameRate=20
Created room room1 on port 8889 (MinPlayers: 2, MaxPlayers: 8)
```

### 3. 连接客户端

**第一个客户端**：
```bash
./lockstep-server client 127.0.0.1 8889 1
```

此时服务器应显示玩家加入但游戏未开始。

**第二个客户端**：
```bash
./lockstep-server client 127.0.0.1 8889 2
```

此时服务器应显示游戏开始运行。

## 配置修改

如果你想修改最小玩家数，可以编辑 `lockstep/types.go` 文件中的 `DefaultRoomConfig` 函数：

```go
func DefaultRoomConfig() *RoomConfig {
    return &RoomConfig{
        MinPlayers: 2,  // 修改这里
        MaxPlayers: 8,
        FrameRate:  30,
    }
}
```

修改后需要重新编译：
```bash
go build -o lockstep-server main.go
```

## 日志文件

测试脚本会生成以下日志文件：

- `server.log` / `demo_server.log` - 服务器日志
- `client1.log` / `demo_client1.log` - 第一个客户端日志
- `client2.log` / `demo_client2.log` - 第二个客户端日志
- `client3.log` - 第三个客户端日志（仅自动化测试）

## 故障排除

### 常见问题

1. **端口被占用**
   ```bash
   pkill -f "lockstep-server"
   ```

2. **编译失败**
   确保在正确的目录下运行，并且 Go 环境配置正确。

3. **客户端连接失败**
   检查服务器是否正常启动，端口是否正确。

### 调试模式

如果需要更详细的日志，可以修改代码中的日志级别或添加更多调试信息。

## 测试结果示例

### 自动化测试输出

```
=== LockStep 最小玩家数功能测试 ===
✅ 服务器配置正确 (最小玩家数=2)
✅ 1个玩家时游戏未开始 (Status=0)
✅ 2个玩家时游戏开始 (Status=2)
✅ 3个玩家时游戏继续运行

📋 服务器日志摘要:
Room room1 created, MinPlayers=2, MaxPlayers=8, FrameRate=20
Player 1 joined room room1
Player 2 joined room room1
Room room1 started with 2 players
Player 3 joined room room1

=== 测试通过 ===
```

### 第三个玩家同步测试输出

```
=== 测试第三个玩家加入时的帧数据同步 ===
✅ 服务器已启动
✅ 第一个客户端已连接
✅ 第二个客户端已连接，游戏开始
✅ 第三个玩家已接收同步帧数据 (10 帧)
✅ 第三个客户端成功接收历史帧数据

📋 服务器日志摘要:
Player 3 joined room room1
Sent 10 sync frames to new player 3 in room room1

=== 测试通过: 第三个玩家加入时正确接收了帧数据同步 ===
```

这表明最小玩家数功能和帧数据同步功能都正常工作。