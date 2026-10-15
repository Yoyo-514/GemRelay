"""使用本机 Chromium 检查真实管理页渲染，无服务器、函数替换或第三方包。"""

import argparse
import base64
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import time
from urllib.request import urlopen

sys.dont_write_bytecode = True
from cdp import DevTools



def main():
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
    browser = args.browser or next(
        (
            path
            for path in candidates
            if path.is_file()
        ),
        None,
    )
    if browser is None:
        parser.error("未找到 Chromium，请用 --browser 指定可执行文件")
    output = args.output or Path(tempfile.mkdtemp(prefix="gw2a-ui-"))
    output.mkdir(parents=True, exist_ok=True)
    target = Path(__file__).with_name("admin-ui.html").resolve().as_uri()
    with tempfile.TemporaryDirectory(prefix="gw2a-browser-") as profile:
        with (output / "browser.log").open("w", encoding="utf-8") as log:
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
                endpoint = next(tab["webSocketDebuggerUrl"] for tab in tabs if tab["type"] == "page")
                client = DevTools(endpoint)
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
                            raise RuntimeError("页面测试未完成")
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
