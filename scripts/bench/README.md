# DNS 基准测试

`udpqps.go` 是一个自包含的 UDP DNS 负载生成器，用于测量 GoDDI 缓存数据面的 QPS。

## 用法

```bash
# 启动 GoDDI（示例：仅 DNS，监听 127.0.0.1:15353）
GODDI_DNS_ENABLED=true \
GODDI_DNS_LISTENERS_UDP_ADDR=127.0.0.1:15353 \
GODDI_DNS_LISTENERS_TCP_ADDR=127.0.0.1:15353 \
./goddi serve

# 预热缓存（发一次查询）
dig @127.0.0.1 -p 15353 www.example.com A +short

# 基准：32 并发、持续 60 秒
go run ./scripts/bench/udpqps.go -server 127.0.0.1:15353 \
    -qname www.example.com. -workers 32 -duration 60s
```

## 常用参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-server` | 目标地址 host:port | `127.0.0.1:53` |
| `-qname` | 查询域名 | `www.example.com.` |
| `-workers` | 并发 worker 数 | `8` |
| `-duration` | 持续时间 | `30s` |
| `-count` | 固定查询数（0 = 按时长） | `0` |

## 参考基线（macOS arm64，缓存命中）

| 场景 | QPS | 成功率 |
|------|-----|--------|
| 单线程 cache hit | ~29,400 | 100% |
| 32 workers, 60s | ~39,700 | 100% |
| 64 workers | ~38,100（客户端侧饱和） | 100% |

> 注意：结果是本机回环压测的上限参考；生产容量规划请结合上游转发延迟与磁盘/日志写入路径重新测量。
