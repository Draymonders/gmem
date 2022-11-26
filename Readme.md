# GMem

尝试 go 做一个简易版的 memory cache 实现

## RESP协议

- [REdis Serialization Protocol](https://redis.io/docs/reference/protocol-spec/)

In RESP, the first byte determines the data type:

- For Simple Strings, the first byte of the reply is "+"
- For Errors, the first byte of the reply is "-"
- For Integers, the first byte of the reply is ":"
- For Bulk Strings, the first byte of the reply is "$"
- For Arrays, the first byte of the reply is "*"


## v0.1

1. 实现 object, list, dict 基础数据结构
2. 实现事件循环 aeEventLoop
3. 参考 [RESP](https://redis.io/docs/reference/protocol-spec/) 协议，实现流式读取&写入
4. 命令表实现 `COMMAND`，`SET`，`GET`
