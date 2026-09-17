(() => {
  const MESSAGE_TYPE = "JAVBOSS_DOWNLOAD_MAGNET";
  const TOAST_ID = "javboss-magnet-download-toast";
  let enabled = false;

  chrome.runtime
    .sendMessage({ type: "JAVBOSS_MAGNET_SETTINGS" })
    .then((settings) => {
      enabled = settings?.enabled === true;
    })
    .catch(() => {});
  chrome.runtime.onMessage.addListener((message) => {
    if (message?.type === "JAVBOSS_MAGNET_SETTINGS_CHANGED") {
      enabled = message.enabled === true;
    }
  });

  function validMagnetURL(value) {
    const candidate = String(value || "").trim();
    if (!candidate || candidate.length > 16384) return "";
    try {
      const parsed = new URL(candidate);
      if (parsed.protocol !== "magnet:") return "";
      const hasInfoHash = parsed.searchParams
        .getAll("xt")
        .some((value) => value.toLowerCase().startsWith("urn:btih:"));
      return hasInfoHash ? candidate : "";
    } catch {
      return "";
    }
  }

  function magnetURLFromText(value) {
    const text = String(value || "").trim();
    if (!text.toLowerCase().startsWith("magnet:?")) return "";
    return validMagnetURL(text.match(/^magnet:\?\S+/i)?.[0]);
  }

  function clickedMagnetURL(event) {
    for (const node of event.composedPath?.() || []) {
      if (["BUTTON", "INPUT", "SELECT", "TEXTAREA"].includes(node?.tagName)) {
        return "";
      }
      const magnetUrl =
        validMagnetURL(node?.tagName === "A" ? node.href : "") ||
        magnetURLFromText(node?.textContent);
      if (magnetUrl) return magnetUrl;
    }
    return validMagnetURL(event.target?.closest?.("a[href]")?.href);
  }

  function javCodeFromText(value) {
    const text = String(value || "");
    const fc2 = text.match(
      /(?:^|[^a-z0-9])(FC2[-_ ]?PPV[-_ ]?\d{5,8})(?:[^a-z0-9]|$)/i,
    );
    if (fc2) return fc2[1].toUpperCase().replace(/[_ ]+/g, "-");
    const standard = text.match(
      /(?:^|[^a-z0-9])([a-z]{2,8})[-_ ]?(\d{2,6})(?:[^a-z0-9]|$)/i,
    );
    if (!standard) return "";
    return `${standard[1].toUpperCase()}-${standard[2]}`;
  }

  function javCodeForMagnet(magnetUrl) {
    let displayName = "";
    try {
      displayName = new URL(magnetUrl).searchParams.get("dn") || "";
    } catch {
      // The magnet URL was already validated; page text remains a fallback.
    }
    return (
      javCodeFromText(displayName) ||
      javCodeFromText(document.querySelector?.("h1")?.textContent) ||
      javCodeFromText(document.title) ||
      javCodeFromText(window.location.pathname)
    );
  }

  function showStatus(message, failed = false) {
    let toast = document.getElementById(TOAST_ID);
    if (!toast) {
      toast = document.createElement("div");
      toast.id = TOAST_ID;
      Object.assign(toast.style, {
        position: "fixed",
        right: "20px",
        bottom: "20px",
        zIndex: "2147483647",
        maxWidth: "360px",
        borderRadius: "8px",
        padding: "10px 14px",
        color: "#fff",
        font: "600 14px/1.4 system-ui, sans-serif",
        boxShadow: "0 8px 24px rgba(15, 23, 42, .3)",
      });
      document.documentElement.appendChild(toast);
    }
    toast.textContent = message;
    toast.style.background = failed ? "#dc2626" : "#2563eb";
    window.clearTimeout(showStatus.timer);
    showStatus.timer = window.setTimeout(
      () => toast.remove(),
      failed ? 5000 : 2500,
    );
  }

  document.addEventListener(
    "click",
    (event) => {
      if (
        !enabled ||
        !event.isTrusted ||
        event.defaultPrevented ||
        event.button !== 0
      )
        return;
      const magnetUrl = clickedMagnetURL(event);
      if (!magnetUrl) return;
      const javCode = javCodeForMagnet(magnetUrl);

      event.preventDefault();
      event.stopImmediatePropagation();
      if (!window.confirm("是否将此磁力链接提交到 JavBoss 下载队列？")) {
        showStatus("已取消提交");
        return;
      }
      showStatus("正在提交到 JavBoss…");
      chrome.runtime
        .sendMessage({ type: MESSAGE_TYPE, magnetUrl, javCode })
        .then(async (response) => {
          if (response?.requiresOverwriteConfirmation) {
            const code = response.code || javCode || "该番号";
            if (
              !window.confirm(
                `番号 ${code} 已存在。覆盖会重新下载并替换同名文件，是否继续？`,
              )
            ) {
              showStatus("已取消覆盖");
              return;
            }
            showStatus("正在创建覆盖下载任务…");
            response = await chrome.runtime.sendMessage({
              type: MESSAGE_TYPE,
              magnetUrl,
              javCode: code,
              overwriteExisting: true,
            });
          }
          if (!response?.ok) throw new Error(response?.error || "提交失败");
          showStatus(
            response?.overwritten
              ? "已提交覆盖下载任务"
              : "已提交到 JavBoss 下载队列",
          );
        })
        .catch((error) => {
          showStatus(`提交到 JavBoss 失败：${error?.message || error}`, true);
        });
    },
    true,
  );
})();
