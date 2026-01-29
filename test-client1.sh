#!/bin/bash

# Client 1 - 测试内存泄漏和 Goroutine 泄漏修复
# 持续运行 5 分钟，监控内存和 goroutine

echo "=== KCPQ Client 1 Test - Memory & Goroutine Leak Fix ==="
echo "Start time: $(date)"
echo "Test duration: 5 minutes"
echo ""

cd /e/Code/KCP/KCPQ

# 启动客户端，持续重连
./kcpq-client-new.exe --server 208.81.129.186:4000 --test-duration 300 2>&1 | tee client1-test.log

echo ""
echo "=== Test completed ==="
echo "End time: $(date)"
