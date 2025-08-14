using System;
using System.Collections.Generic;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
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

    public class CSharpClient
    {
        private KcpClient client;
        private readonly List<TestMessage> receivedMessages = new List<TestMessage>();
        private int messageCounter = 0;
        private bool connected = false;
        private readonly Dictionary<string, bool> testResults = new Dictionary<string, bool>();
        private readonly List<string> pendingTests = new List<string>
        {
            "PING",
            "ECHO_TEST",
            "RELIABLE_TEST",
            "UNRELIABLE_TEST",
            "LARGE_MESSAGE_TEST"
        };

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

        public async Task<bool> ConnectAsync(string hostname = "127.0.0.1", ushort port = 7777)
        {
            Console.WriteLine($"[C# Client] Connecting to {hostname}:{port}...");
            
            client = new KcpClient(
                OnConnected,
                OnData,
                OnDisconnected,
                OnError,
                config
            );
            
            client.Connect(hostname, port);
            
            // 等待连接建立
            for (int i = 0; i < 100; i++) // 最多等待10秒
            {
                client.Tick();
                if (connected)
                {
                    Console.WriteLine("[C# Client] Connected successfully!");
                    return true;
                }
                await Task.Delay(100);
            }
            
            Console.WriteLine("[C# Client] Connection timeout");
            return false;
        }

        public void Disconnect()
        {
            Console.WriteLine("[C# Client] Disconnecting...");
            client?.Disconnect();
            connected = false;
        }

        public void Update()
        {
            client?.Tick();
        }

        private void OnConnected()
        {
            connected = true;
            Console.WriteLine($"[C# Client] Connected at {DateTime.Now:HH:mm:ss.fff}");
        }

        private void OnData(ArraySegment<byte> data, KcpChannel channel)
        {
            try
            {
                var json = System.Text.Encoding.UTF8.GetString(data);
                var message = JsonSerializer.Deserialize<TestMessage>(json);
                message.Channel = channel;
                
                receivedMessages.Add(message);
                
                Console.WriteLine($"[C# Client] Received: ID={message.Id}, Content='{message.Content}', Channel={channel}, Size={data.Count} bytes");
                
                // 处理测试响应
                HandleTestResponse(message);
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[C# Client] Error parsing message: {ex.Message}");
            }
        }

        private void OnDisconnected()
        {
            connected = false;
            Console.WriteLine($"[C# Client] Disconnected at {DateTime.Now:HH:mm:ss.fff}");
            Console.WriteLine($"[C# Client] Total messages received: {receivedMessages.Count}");
        }

        private void OnError(ErrorCode error, string reason)
        {
            Console.WriteLine($"[C# Client] Error: {error}, Reason: {reason}");
        }

        private void HandleTestResponse(TestMessage message)
        {
            switch (message.Content)
            {
                case "PONG":
                    testResults["PING"] = true;
                    Console.WriteLine("[C# Client] ✓ PING test passed");
                    break;
                    
                case var content when content.StartsWith("ECHO:"):
                    testResults["ECHO_TEST"] = true;
                    Console.WriteLine("[C# Client] ✓ ECHO test passed");
                    break;
                    
                case var content when content.StartsWith("Reliable"):
                    if (!testResults.ContainsKey("RELIABLE_TEST") || !testResults["RELIABLE_TEST"])
                    {
                        testResults["RELIABLE_TEST"] = true;
                        Console.WriteLine("[C# Client] ✓ RELIABLE test passed");
                    }
                    break;
                    
                case var content when content.StartsWith("Unreliable"):
                    if (!testResults.ContainsKey("UNRELIABLE_TEST") || !testResults["UNRELIABLE_TEST"])
                    {
                        testResults["UNRELIABLE_TEST"] = true;
                        Console.WriteLine("[C# Client] ✓ UNRELIABLE test passed");
                    }
                    break;
                    
                case var content when content.Length >= 1000: // 大消息测试
                    testResults["LARGE_MESSAGE_TEST"] = true;
                    Console.WriteLine("[C# Client] ✓ LARGE_MESSAGE test passed");
                    break;
            }
        }

        public void SendMessage(TestMessage message)
        {
            if (!connected)
            {
                Console.WriteLine("[C# Client] Not connected, cannot send message");
                return;
            }
            
            try
            {
                var json = JsonSerializer.Serialize(message);
                var data = System.Text.Encoding.UTF8.GetBytes(json);
                client.Send(new ArraySegment<byte>(data), message.Channel);
                Console.WriteLine($"[C# Client] Sent: ID={message.Id}, Content='{message.Content}', Channel={message.Channel}, Size={data.Length} bytes");
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[C# Client] Error sending message: {ex.Message}");
            }
        }

        public async Task RunTestsAsync()
        {
            Console.WriteLine("[C# Client] Starting automated tests...");
            
            // 等待欢迎消息
            await Task.Delay(500);
            Update();
            
            // 运行所有测试
            foreach (var testName in pendingTests)
            {
                Console.WriteLine($"[C# Client] Running test: {testName}");
                
                var message = new TestMessage
                {
                    Id = ++messageCounter,
                    Content = testName,
                    Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                    Channel = KcpChannel.Reliable
                };
                
                if (testName == "UNRELIABLE_TEST")
                {
                    message.Channel = KcpChannel.Unreliable;
                }
                
                SendMessage(message);
                
                // 等待响应
                for (int i = 0; i < 50; i++) // 最多等待5秒
                {
                    Update();
                    if (testResults.ContainsKey(testName) && testResults[testName])
                    {
                        break;
                    }
                    await Task.Delay(100);
                }
                
                if (!testResults.ContainsKey(testName) || !testResults[testName])
                {
                    Console.WriteLine($"[C# Client] ✗ Test {testName} failed or timed out");
                }
                
                await Task.Delay(500); // 测试间隔
            }
        }

        public async Task RunInteractiveModeAsync()
        {
            Console.WriteLine("[C# Client] Interactive mode started");
            Console.WriteLine("Commands: ping, echo, reliable, unreliable, large, disconnect, quit");
            
            while (true)
            {
                Update();
                
                Console.Write("> ");
                var input = Console.ReadLine()?.Trim().ToLower();
                
                switch (input)
                {
                    case "quit" or "q":
                        return;
                        
                    case "ping":
                        SendMessage(new TestMessage
                        {
                            Id = ++messageCounter,
                            Content = "PING",
                            Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                            Channel = KcpChannel.Reliable
                        });
                        break;
                        
                    case "echo":
                        SendMessage(new TestMessage
                        {
                            Id = ++messageCounter,
                            Content = "ECHO_TEST",
                            Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                            Channel = KcpChannel.Reliable
                        });
                        break;
                        
                    case "reliable":
                        SendMessage(new TestMessage
                        {
                            Id = ++messageCounter,
                            Content = "RELIABLE_TEST",
                            Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                            Channel = KcpChannel.Reliable
                        });
                        break;
                        
                    case "unreliable":
                        SendMessage(new TestMessage
                        {
                            Id = ++messageCounter,
                            Content = "UNRELIABLE_TEST",
                            Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                            Channel = KcpChannel.Unreliable
                        });
                        break;
                        
                    case "large":
                        SendMessage(new TestMessage
                        {
                            Id = ++messageCounter,
                            Content = "LARGE_MESSAGE_TEST",
                            Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                            Channel = KcpChannel.Reliable
                        });
                        break;
                        
                    case "disconnect":
                        SendMessage(new TestMessage
                        {
                            Id = ++messageCounter,
                            Content = "DISCONNECT_TEST",
                            Timestamp = DateTimeOffset.Now.ToUnixTimeMilliseconds(),
                            Channel = KcpChannel.Reliable
                        });
                        break;
                        
                    default:
                        Console.WriteLine("Unknown command. Available: ping, echo, reliable, unreliable, large, disconnect, quit");
                        break;
                }
                
                await Task.Delay(10);
            }
        }

        public void PrintTestResults()
        {
            Console.WriteLine("\n[C# Client] Test Results:");
            Console.WriteLine("=========================");
            int passed = 0;
            int total = pendingTests.Count;
            
            foreach (var testName in pendingTests)
            {
                if (testResults.ContainsKey(testName) && testResults[testName])
                {
                    Console.WriteLine($"✓ {testName}: PASSED");
                    passed++;
                }
                else
                {
                    Console.WriteLine($"✗ {testName}: FAILED");
                }
            }
            
            Console.WriteLine($"\nSummary: {passed}/{total} tests passed");
            Console.WriteLine($"Total messages received: {receivedMessages.Count}");
        }
    }

    class Program
    {
        static async Task Main(string[] args)
        {
            // 解析命令行参数
            string hostname = "127.0.0.1";
            ushort port = 7777;
            bool autoTest = false;
            
            for (int i = 0; i < args.Length; i++)
            {
                switch (args[i])
                {
                    case "-h" or "--host":
                        if (i + 1 < args.Length)
                            hostname = args[++i];
                        break;
                        
                    case "-p" or "--port":
                        if (i + 1 < args.Length && ushort.TryParse(args[++i], out var p))
                            port = p;
                        break;
                        
                    case "-a" or "--auto":
                        autoTest = true;
                        break;
                }
            }
            
            var client = new CSharpClient();
            
            try
            {
                // 连接到服务器
                if (!await client.ConnectAsync(hostname, port))
                {
                    Console.WriteLine("[C# Client] Failed to connect to server");
                    return;
                }
                
                if (autoTest)
                {
                    // 自动测试模式
                    await client.RunTestsAsync();
                    client.PrintTestResults();
                }
                else
                {
                    // 交互模式
                    await client.RunInteractiveModeAsync();
                }
            }
            catch (Exception ex)
            {
                Console.WriteLine($"[C# Client] Fatal error: {ex.Message}");
            }
            finally
            {
                client.Disconnect();
                Console.WriteLine("[C# Client] Exiting...");
            }
        }
    }
}