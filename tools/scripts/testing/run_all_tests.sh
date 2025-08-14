#!/bin/bash

# KCP2K 跨语言兼容性测试 - 主测试脚本
# 运行所有测试场景

# 设置工作目录到项目根目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
cd "$PROJECT_ROOT"

echo "========================================"
echo "KCP2K Cross-Language Compatibility Tests"
echo "Running All Test Scenarios"
echo "========================================"

# 配置
RESULT_DIR="test_results"
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
SUMMARY_FILE="${RESULT_DIR}/test_summary_${TIMESTAMP}.txt"

# 创建结果目录
mkdir -p "$RESULT_DIR"

# 初始化测试结果
TOTAL_SCENARIOS=0
PASSED_SCENARIOS=0
FAILED_SCENARIOS=0

echo "Test started at: $(date)"
echo "Results directory: $RESULT_DIR"
echo "Summary file: $SUMMARY_FILE"
echo ""

# 创建摘要文件
cat > "$SUMMARY_FILE" << EOF
KCP2K Cross-Language Compatibility Test Summary
Timestamp: $(date)

=== Test Scenarios ===
EOF

# 运行测试场景1：C# 服务端 + Go 客户端
echo "=========================================="
echo "Scenario 1: C# Server + Go Client"
echo "=========================================="
TOTAL_SCENARIOS=$((TOTAL_SCENARIOS + 1))

if ./tools/scripts/testing/run_csharp_server_go_client.sh; then
    echo "✓ Scenario 1 PASSED"
    PASSED_SCENARIOS=$((PASSED_SCENARIOS + 1))
    echo "1. C# Server + Go Client: PASSED" >> "$SUMMARY_FILE"
else
    echo "✗ Scenario 1 FAILED"
    FAILED_SCENARIOS=$((FAILED_SCENARIOS + 1))
    echo "1. C# Server + Go Client: FAILED" >> "$SUMMARY_FILE"
fi

echo ""
echo "Waiting 5 seconds before next test..."
sleep 5

# 运行测试场景2：Go 服务端 + C# 客户端
echo "=========================================="
echo "Scenario 2: Go Server + C# Client"
echo "=========================================="
TOTAL_SCENARIOS=$((TOTAL_SCENARIOS + 1))

if ./tools/scripts/testing/run_go_server_csharp_client.sh; then
    echo "✓ Scenario 2 PASSED"
    PASSED_SCENARIOS=$((PASSED_SCENARIOS + 1))
    echo "2. Go Server + C# Client: PASSED" >> "$SUMMARY_FILE"
else
    echo "✗ Scenario 2 FAILED"
    FAILED_SCENARIOS=$((FAILED_SCENARIOS + 1))
    echo "2. Go Server + C# Client: FAILED" >> "$SUMMARY_FILE"
fi

# 生成最终摘要
echo "" >> "$SUMMARY_FILE"
echo "=== Final Results ===" >> "$SUMMARY_FILE"
echo "Total Scenarios: $TOTAL_SCENARIOS" >> "$SUMMARY_FILE"
echo "Passed: $PASSED_SCENARIOS" >> "$SUMMARY_FILE"
echo "Failed: $FAILED_SCENARIOS" >> "$SUMMARY_FILE"
echo "Success Rate: $(( PASSED_SCENARIOS * 100 / TOTAL_SCENARIOS ))%" >> "$SUMMARY_FILE"
echo "Test completed at: $(date)" >> "$SUMMARY_FILE"

# 显示最终结果
echo ""
echo "========================================"
echo "Final Test Results"
echo "========================================"
echo "Total Scenarios: $TOTAL_SCENARIOS"
echo "Passed: $PASSED_SCENARIOS"
echo "Failed: $FAILED_SCENARIOS"
echo "Success Rate: $(( PASSED_SCENARIOS * 100 / TOTAL_SCENARIOS ))%"
echo ""
echo "Detailed summary saved to: $SUMMARY_FILE"

# 列出所有生成的文件
echo ""
echo "Generated test files:"
ls -la "$RESULT_DIR"/*${TIMESTAMP}* 2>/dev/null || echo "No test files found"

# 确定退出代码
if [ $FAILED_SCENARIOS -eq 0 ]; then
    echo ""
    echo "🎉 ALL TESTS PASSED! KCP2K cross-language compatibility verified."
    exit 0
else
    echo ""
    echo "❌ Some tests failed. KCP2K cross-language compatibility issues detected."
    echo "Check individual test reports for details."
    exit 1
fi