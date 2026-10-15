// boot — try fetching stats; if 401, login page
(async () => {
  try {
    const response = await fetch('api/stats?hours=1', { credentials: 'include' });
    if (response.ok) renderShell();
    else renderLogin();
  } catch (error) {
    renderLogin();
    document.getElementById('loginErr').textContent = '无法连接服务，请检查网络后重试。';
  }
})();
