using System;
using System.Collections.Generic;
using System.Net;
using System.Threading;
using kcp2k;

namespace CrossLanguageTests
{
    public class TestMessage
    {
        public int Id { get; set; }
        public string Content { get; set; }
        public long Timestamp { get; set; }
        public KcpChannel Channel { get; set; }
    }

    public class CSharpServer
    {
        private KcpServer server;
        private readonly Dictionary<int, DateTime> connectionTimes = new Dictionary<int, DateTime>();
        private readonly Dictionary<int, List<TestMessage>> receivedMessages = new Dictionary<int, List<TestMessage>>();
        private int messageCounter = 0;
        private bool isRunning = false;

        // 测试配置 - 与Go版本保持一致
        private readonly KcpConfig config = new KcpConfig
        {
            DualMode = false,
            RecvBufferSize = 1024 * 1024 * 7,
            SendBufferSize = 1024 * 1024 * 7,
            Mtu = 1400,
            NoDelay = true,
            Interval = 1,
            FastResend = 0,
            CongestionWindow = false,
            SendWindowSize = 32000,
            ReceiveWindowSize = 32000,
            Timeout = 10000,
            MaxRetransmits = 40
        };

        public void Start(ushort port = 7777)
        {
            Console.WriteLine($"[C# Server] Starting server on port {port}...");
            
            server = new KcpServer(
                OnConnected,
                OnData,
                OnDisconnected,
                OnError,
                config
            );

            server.Start(port);
            isRunning = true;
            Console.WriteLine($"[C# Server] Server started successfully on port {port}");
        }

        public void Stop()
        {
            Console.WriteLine("[C# Server] Stopping server...");
            isRunning = false;
            server?.Stop();
            Console.WriteLine("[C# Server] Server stopped");
        }

        public void Update()
        {
            server?.Tick();
        }

        private void OnConnected(int connectionId)
        {
            connectionTimes[connectionId] = DateTime.Now;
            receivedMessages[connectionId] = new List<TestMessage>();
            Console.WriteLine($"[C# Server] Client {connectionId} connected at {DateTime.Now:HH:mm:ss.fff}");
            
            // 发送欢迎消息
            var welcomeMsg = new TestMessage
            {
                Id = ++messageCounter,
                Content = "Welcome from C# Server!",
                Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                Channel = KcpChannel.Reliable
            };
            
            SendMessage(connectionId, welcomeMsg);
        }

        private void OnData(int connectionId, ArraySegment<byte> data, KcpChannel channel)
        {
            try
            {
                var message = System.Text.Json.JsonSerializer.Deserialize<TestMessage>(data);
                message.Channel = channel;
                
                receivedMessages[connectionId].Add(message);
                
                Console.WriteLine($"[C# Server] Received from {connectionId}: ID={message.Id}, Content='{message.Content}', Channel={channel}, Size={data.Count} bytes");
                
                // 根据消息内容执行不同的测试响应
                HandleTestMessage(connectionId, message);
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[C# Server] Error parsing message from {connectionId}: {ex.Message}");
            }
        }

        private void OnDisconnected(int connectionId)
        {
            if (connectionTimes.TryGetValue(connectionId, out var connectTime))
            {
                var duration = DateTime.Now - connectTime;
                Console.WriteLine($"[C# Server] Client {connectionId} disconnected after {duration.TotalSeconds:F2} seconds");
                connectionTimes.Remove(connectionId);
            }
            
            if (receivedMessages.ContainsKey(connectionId))
            {
                Console.WriteLine($"[C# Server] Client {connectionId} received {receivedMessages[connectionId].Count} messages total");
                receivedMessages.Remove(connectionId);
            }
        }

        private void OnError(int connectionId, ErrorCode error, string reason)
        {
            Console.WriteLine($"[C# Server] Error on connection {connectionId}: {error}, Reason: {reason}");
        }

        private void HandleTestMessage(int connectionId, TestMessage message)
        {
            switch (message.Content)
            {
                case "PING":
                    // 响应PING消息
                    var pongMsg = new TestMessage
                    {
                        Id = ++messageCounter,
                        Content = "PONG",
                        Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                        Channel = message.Channel
                    };
                    SendMessage(connectionId, pongMsg);
                    break;
                    
                case "ECHO_TEST":
                    // 回显测试
                    var echoMsg = new TestMessage
                    {
                        Id = message.Id, // 保持相同ID
                        Content = $"ECHO: {message.Content}",
                        Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                        Channel = message.Channel
                    };
                    SendMessage(connectionId, echoMsg);
                    break;
                    
                case "RELIABLE_TEST":
                    // 可靠消息测试
                    for (int i = 0; i < 5; i++)
                    {
                        var reliableMsg = new TestMessage
                        {
                            Id = ++messageCounter,
                            Content = $"Reliable message {i + 1}/5",
                            Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                            Channel = KcpChannel.Reliable
                        };
                        SendMessage(connectionId, reliableMsg);
                    }
                    break;
                    
                case "UNRELIABLE_TEST":
                    // 不可靠消息测试
                    for (int i = 0; i < 5; i++)
                    {
                        var unreliableMsg = new TestMessage
                        {
                            Id = ++messageCounter,
                            Content = $"Unreliable message {i + 1}/5",
                            Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                            Channel = KcpChannel.Unreliable
                        };
                        SendMessage(connectionId, unreliableMsg);
                    }
                    break;
                    
                case "LARGE_MESSAGE_TEST":
                    // 大消息测试
                    var largeContent = new string('A', 1000); // 1KB消息
                    var largeMsg = new TestMessage
                    {
                        Id = ++messageCounter,
                        Content = largeContent,
                        Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                        Channel = KcpChannel.Reliable
                    };
                    SendMessage(connectionId, largeMsg);
                    break;
                    
                case "DISCONNECT_TEST":
                    // 断开连接测试
                    Console.WriteLine($"[C# Server] Disconnecting client {connectionId} as requested");
                    server.Disconnect(connectionId);
                    break;
                    
                default:
                    // 默认回显
                    var defaultMsg = new TestMessage
                    {
                        Id = ++messageCounter,
                        Content = $"Server received: {message.Content}",
                        Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                        Channel = message.Channel
                    };
                    SendMessage(connectionId, defaultMsg);
                    break;
            }
        }

        private void SendMessage(int connectionId, TestMessage message)
        {
            try
            {
                var json = System.Text.Json.JsonSerializer.Serialize(message);
                var data = System.Text.Encoding.UTF8.GetBytes(json);
                server.Send(connectionId, new ArraySegment<byte>(data), message.Channel);
                Console.WriteLine($"[C# Server] Sent to {connectionId}: ID={message.Id}, Content='{message.Content}', Channel={message.Channel}, Size={data.Length} bytes");
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[C# Server] Error sending message to {connectionId}: {ex.Message}");
            }
        }

        public void PrintStats()
        {
            Console.WriteLine($"[C# Server] Active connections: {connectionTimes.Count}");
            foreach (var kvp in receivedMessages)
            {
                Console.WriteLine($"[C# Server] Connection {kvp.Key}: {kvp.Value.Count} messages received");
            }
        }
    }

    class Program
    {
        static void Main(string[] args)
        {
            var server = new CSharpServer();
            
            // 解析命令行参数
            ushort port = 7777;
            if (args.Length > 0 && ushort.TryParse(args[0], out var parsedPort))
            {
                port = parsedPort;
            }
            
            try
            {
                server.Start(port);
                
                Console.WriteLine("Server running for 60 seconds...");
                
                // 主循环 - 运行60秒后自动退出
                var startTime = DateTime.Now;
                var timeout = TimeSpan.FromSeconds(60);
                
                while (DateTime.Now - startTime < timeout)
                {
                    server.Update();
                    Thread.Sleep(1); // 1ms间隔，与配置一致
                }
                
                Console.WriteLine("Server timeout reached, shutting down...");
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[C# Server] Fatal error: {ex.Message}");
            }
            finally
            {
                server.Stop();
            }
        }
    }
}