"""使用本机 Chromium 验证管理页渲染和额度轮询行为。"""

import argparse
import base64
import json
import shutil
import subprocess
import sys
import tempfile
import threading
import time
from contextlib import contextmanager
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlsplit
from urllib.request import urlopen

sys.dont_write_bytecode = True


@contextmanager
def quota_server():
    state = {"mode": "success", "calls": 0, "active": 0, "peak": 0, "forced": 0}
    lock = threading.Lock()
    release = threading.Event()
    items = [{"id": n, "label": f"account-{n}", "status": "enabled", "health": "ok"} for n in range(1, 6)]

    class Handler(SimpleHTTPRequestHandler):
        def log_message(self, format, *args):
            pass

        def send_json(self, value):
            body = json.dumps(value).encode()
            try:
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
            except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
                pass

        def do_GET(self):
            url = urlsplit(self.path)
            if url.path.endswith("/api/cookies"):
                self.send_json({"items": items, "total": 5, "enabled": 5, "quota_poll_interval_sec": 0.08})
            elif url.path.endswith("/api/proxies"):
                self.send_json({"items": []})
            elif url.path.endswith("/api/quota"):
                query = parse_qs(url.query)
                with lock:
                    mode = state["mode"]
                    state["calls"] += 1
                    state["active"] += 1
                    state["peak"] = max(state["peak"], state["active"])
                    state["forced"] += int(query.get("refresh") == ["1"])
                try:
                    if mode == "blocked":
                        release.wait(5)
                    else:
                        time.sleep(0.04)
                    result = {"account_id": int(query["account_id"][0]), "ok": mode != "failure"}
                    if mode == "failure":
                        result["error"] = "额度查询连接超时"
                    else:
                        result.update({"cached": True, "fetched_at": int(time.time()),
                                       "five_hour": {"remain_percent": 77 if mode == "recovered" else 37}})
                    self.send_json(result)
                finally:
                    with lock:
                        state["active"] -= 1
            else:
                super().do_GET()

    root = Path(__file__).resolve().parents[2]
    server = ThreadingHTTPServer(("127.0.0.1", 0), partial(Handler, directory=str(root)))
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}", state, lock, release
    finally:
        release.set()
        server.shutdown()
        server.server_close()
        worker.join()


def wait_for(client, expression):
    deadline = time.monotonic() + 10
    while not client.evaluate(expression):
        if time.monotonic() > deadline:
            raise AssertionError(f"轮询验证超时：{expression}")
        time.sleep(0.02)


def check_quota_poll(client, target_id):
    with quota_server() as (origin, state, lock, release):
        client.call("Page.navigate", {"url": origin + "/tests/web/admin-ui.html?surface=cookies"})
        wait_for(client, "document.documentElement.dataset.testStatus === 'passed'")
        client.evaluate("document.querySelector('nav [data-page=cookies]').click()")
        wait_for(client, "document.querySelectorAll('[data-quota-id]').length === 5 && document.querySelectorAll('[data-quota-id] .quota-remaining').length === 5")
        assert client.evaluate("document.querySelectorAll('.quota-stale').length") == 0
        with lock:
            first = state["calls"]
        deadline = time.monotonic() + 5
        while state["calls"] <= first:
            assert time.monotonic() < deadline, "应按间隔自动查询下一轮"
            time.sleep(0.02)
        with lock:
            assert state["peak"] <= 3, f"额度查询并发={state['peak']}"
            state["mode"] = "failure"
        wait_for(client, "document.querySelectorAll('.quota-stale').length === 5")
        assert client.evaluate("document.querySelector('.quota-content').textContent.includes('37%')")
        assert client.evaluate("getComputedStyle(document.querySelector('.quota-stale')).color !== getComputedStyle(document.querySelector('.quota-meta')).color")
        with lock:
            state["mode"] = "recovered"
        wait_for(client, "document.querySelectorAll('.quota-stale').length === 0 && document.querySelector('.quota-content').textContent.includes('77%')")
        client.evaluate("document.querySelector('[data-quota-id] button').click()")
        deadline = time.monotonic() + 5
        while state["forced"] == 0:
            assert time.monotonic() < deadline, "手动刷新应跳过服务端缓存"
            time.sleep(0.02)

        # 切换真实浏览器标签页，覆盖 visibilitychange 的暂停和恢复。
        other = client.call("Target.createTarget", {"url": "about:blank"})["targetId"]
        try:
            client.call("Target.activateTarget", {"targetId": other})
            wait_for(client, "document.hidden")
            time.sleep(0.1)
            with lock:
                paused = state["calls"]
            time.sleep(0.25)
            with lock:
                assert state["calls"] == paused, "隐藏标签页应暂停额度查询"
        finally:
            client.call("Target.activateTarget", {"targetId": target_id})
            client.call("Target.closeTarget", {"targetId": other})
        wait_for(client, "!document.hidden")
        deadline = time.monotonic() + 5
        while state["calls"] <= paused:
            assert time.monotonic() < deadline, "可见时应恢复查询"
            time.sleep(0.02)

        with lock:
            state["mode"] = "blocked"
        deadline = time.monotonic() + 5
        while state["active"] == 0:
            assert time.monotonic() < deadline, "应进入在途请求场景"
            time.sleep(0.02)
        client.evaluate("void switchPage('proxies')")
        wait_for(client, "currentPage === 'proxies' && document.querySelectorAll('[data-quota-id]').length === 0")
        with lock:
            stopped = state["calls"]
        release.set()
        time.sleep(0.25)
        with lock:
            assert state["calls"] == stopped, "离页后查询应结束"
        assert client.evaluate("document.querySelectorAll('.quota-content').length") == 0
        print("quota-poll: passed (轮询、三路并发、缓存提示、恢复、手动刷新、隐藏暂停、离页取消)")


def main():
    from cdp import DevTools

    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--browser", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--keep-artifacts", action="store_true", help="保留检查截图供人工查看；默认运行后清理")
    args = parser.parse_args()
    candidates = [
        Path("C:/Program Files/Google/Chrome/Application/chrome.exe"),
        Path("C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"),
    ]
    for name in ("chromium", "google-chrome", "msedge"):
        executable = shutil.which(name)
        if executable:
            candidates.append(Path(executable))
    browser = args.browser or next((path for path in candidates if path.is_file()), None)
    if browser is None:
        parser.error("请用 --browser 指定 Chromium 可执行文件")
    output = args.output or Path(tempfile.mkdtemp(prefix="gemrelay-ui-"))
    output.mkdir(parents=True, exist_ok=True)
    target = Path(__file__).with_name("admin-ui.html").resolve().as_uri()
    with (
        tempfile.TemporaryDirectory(prefix="gemrelay-browser-") as profile,
        (output / "browser.log").open("w", encoding="utf-8") as log,
    ):
        process = subprocess.Popen(
            [
                str(browser), "--headless=new", "--disable-gpu", "--no-first-run",
                "--disable-background-networking", "--allow-file-access-from-files",
                f"--user-data-dir={profile}", "--remote-debugging-port=0", "about:blank",
            ],
            stdout=log, stderr=log,
        )
        client = None
        try:
            active_port = Path(profile) / "DevToolsActivePort"
            deadline = time.monotonic() + 20
            while not active_port.is_file():
                if process.poll() is not None or time.monotonic() > deadline:
                    raise RuntimeError(f"浏览器启动失败，详见 {output}")
                time.sleep(0.1)
            port = active_port.read_text().splitlines()[0]
            with urlopen(f"http://127.0.0.1:{port}/json", timeout=10) as response:
                tabs = json.load(response)
            tab = next(tab for tab in tabs if tab["type"] == "page")
            client = DevTools(tab["webSocketDebuggerUrl"])
            client.call("Page.enable")
            captures = [
                (f"{device}-{surface}", width, height, surface)
                for device, width, height in [("desktop", 1440, 1000), ("mobile", 390, 844)]
                for surface in ("cookies", "settings", "proxies")
            ]
            for name, width, height, surface in captures:
                client.call("Emulation.setDeviceMetricsOverride", {
                    "width": width, "height": height, "deviceScaleFactor": 1, "mobile": width <= 600,
                })
                query = f"?capture={name}&surface={surface}"
                client.call("Page.navigate", {"url": target + query})
                deadline = time.monotonic() + 15
                while True:
                    state = client.evaluate(
                        f"location.search === '{query}' && document.readyState === 'complete' && document.documentElement.dataset.testStatus"
                    )
                    if state in ("passed", "failed"):
                        break
                    if time.monotonic() > deadline:
                        raise RuntimeError("页面测试超时")
                    time.sleep(0.1)
                details = client.evaluate("({...document.documentElement.dataset})")
                dom = client.evaluate("document.documentElement.outerHTML")
                (output / f"{name}.html").write_text(dom, encoding="utf-8")
                if state != "passed" or details.get("testViewport") != f"{width}x{height}":
                    raise RuntimeError(f"{name} 验证失败: {details}; 详见 {output}")
                screenshot = client.call("Page.captureScreenshot", {"format": "png"})
                capture = output / f"{name}.png"
                capture.write_bytes(base64.b64decode(screenshot["data"]))
                print(f"{name}: passed ({width}x{height}); screenshot={capture}")
            check_quota_poll(client, tab["id"])
        finally:
            if client:
                client.close()
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
    if args.keep_artifacts or args.output:
        print(f"artifacts={output}")
    else:
        shutil.rmtree(output)
        print("检查通过，截图与日志已清理")


if __name__ == "__main__":
    main()
