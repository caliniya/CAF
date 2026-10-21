import { createClient, statusRows, canBeginLogin, canSendGroupText, errorMessage } from "./bridge.mjs";

const list = document.querySelector("#status");
const message = document.querySelector("#message");
const login = document.querySelector("#login");
const cancel = document.querySelector("#cancel");
const refresh = document.querySelector("#refresh");
const qr = document.querySelector("#qr");
const form = document.querySelector("#send-form");
const send = document.querySelector("#send");
let client;
let updating = false;
let sending = false;

function render(status) {
  list.replaceChildren();
  for (const [label, value] of statusRows(status)) {
    const term = document.createElement("dt");
    const description = document.createElement("dd");
    term.textContent = label;
    description.textContent = value;
    list.append(term, description);
  }
  login.disabled = !canBeginLogin(status);
  cancel.disabled = !status.ready || !["starting", "waiting", "scanned", "logging_in", "online"].includes(status.qq?.phase);
  send.disabled = sending || !canSendGroupText(status);
  const code = status.qq?.qrCode;
  // 只接受后端生成的 PNG 数据，不允许任意外部 URL 或脚本地址。
  qr.hidden = !code?.startsWith("data:image/png;base64,");
  if (qr.hidden) qr.removeAttribute("src"); else if (qr.getAttribute("src") !== code) qr.src = code;
  message.textContent = status.qq?.error || status.lastError || (status.ready ? "就绪；二维码和凭据仅保存在本机，请勿发到群中。" : "正在初始化");
}

async function update() {
  if (updating || !client) return;
  updating = true;
  try { render(await client.status()); } catch (error) { message.textContent = errorMessage(error); }
  finally { updating = false; }
}
refresh.addEventListener("click", update);
login.addEventListener("click", async () => {
  login.disabled = true;
  try { render(await client.beginLogin()); } catch (error) { await update(); message.textContent = errorMessage(error); }
});
cancel.addEventListener("click", async () => {
  cancel.disabled = true;
  try { render(await client.cancelLogin()); } catch (error) { message.textContent = errorMessage(error); }
});
form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const group = Number(document.querySelector("#group").value);
  const text = document.querySelector("#text").value;
  if (!Number.isInteger(group) || group <= 0 || group > 4294967295 || !text.trim()) {
    message.textContent = "请填写有效群号和非空文本"; return;
  }
  sending = true; send.disabled = true;
  try { render(await client.sendGroupText(group, text)); message.textContent = "发送调用成功；未执行自动重试。"; }
  catch (error) { message.textContent = errorMessage(error); }
  finally { sending = false; }
});
try {
  client = createClient(window.go);
  await update();
  // 只轮询本地状态，不会自动登录或发送消息。
  const timer = setInterval(update, 1500);
  window.addEventListener("beforeunload", () => clearInterval(timer), { once: true });
} catch (error) { refresh.disabled = true; message.textContent = errorMessage(error); }
