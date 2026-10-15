"""仅供本地浏览器验证使用的最小 DevTools 客户端，不提供 HTTP 服务。"""

import json
import os
import socket
import struct
from urllib.parse import urlsplit


class DevTools:
    def __init__(self, endpoint):
        url = urlsplit(endpoint)
        self.socket = socket.create_connection((url.hostname, url.port), timeout=20)
        self.reader = self.socket.makefile("rb")
        request = (
            f"GET {url.path} HTTP/1.1\r\nHost: {url.netloc}\r\n"
            "Upgrade: websocket\r\nConnection: Upgrade\r\n"
            "Sec-WebSocket-Key: Z3cyYS11aS10ZXN0LWNkcA==\r\n"
            "Sec-WebSocket-Version: 13\r\n\r\n"
        )
        self.socket.sendall(request.encode())
        status = self.reader.readline()
        if b"101" not in status:
            raise RuntimeError(f"DevTools 连接失败: {status!r}")
        while self.reader.readline() != b"\r\n":
            pass
        self.sequence = 0

    def send(self, payload, opcode=1):
        mask = os.urandom(4)
        length = len(payload)
        head = bytes([0x80 | opcode])
        if length < 126:
            head += bytes([0x80 | length])
        elif length < 65536:
            head += bytes([0x80 | 126]) + struct.pack("!H", length)
        else:
            head += bytes([0x80 | 127]) + struct.pack("!Q", length)
        encoded = bytes(value ^ mask[i % 4] for i, value in enumerate(payload))
        self.socket.sendall(head + mask + encoded)

    def receive(self):
        message = bytearray()
        while True:
            head = self.reader.read(2)
            if len(head) != 2:
                raise RuntimeError("DevTools 连接提前关闭")
            first, second = head
            length = second & 0x7F
            if length == 126:
                length = struct.unpack("!H", self.reader.read(2))[0]
            elif length == 127:
                length = struct.unpack("!Q", self.reader.read(8))[0]
            mask = self.reader.read(4) if second & 0x80 else None
            payload = self.reader.read(length)
            if mask:
                payload = bytes(value ^ mask[i % 4] for i, value in enumerate(payload))
            opcode = first & 0x0F
            if opcode == 8:
                raise RuntimeError("DevTools 关闭了连接")
            if opcode == 9:
                self.send(payload, 10)
                continue
            if opcode in (0, 1):
                message.extend(payload)
                if first & 0x80:
                    return json.loads(message)

    def call(self, method, params=None):
        self.sequence += 1
        sequence = self.sequence
        self.send(json.dumps({"id": sequence, "method": method, "params": params or {}}).encode())
        while True:
            result = self.receive()
            if result.get("id") == sequence:
                if "error" in result:
                    raise RuntimeError(result["error"])
                return result.get("result", {})

    def evaluate(self, expression):
        result = self.call("Runtime.evaluate", {"expression": expression, "returnByValue": True})
        if "exceptionDetails" in result:
            raise RuntimeError(result["exceptionDetails"])
        return result.get("result", {}).get("value")

    def close(self):
        self.reader.close()
        self.socket.close()
