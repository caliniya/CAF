import { createClient, statusRows, canLoadNative, errorMessage } from "./bridge.mjs";

const list = document.querySelector("#status");
const message = document.querySelector("#message");
const load = document.querySelector("#load");
const refresh = document.querySelector("#refresh");
let client;

function render(status) {
  list.replaceChildren();
  for (const [label, value] of statusRows(status)) {
    const term = document.createElement("dt");
    const description = document.createElement("dd");
    term.textContent = label;
    description.textContent = value;
    list.append(term, description);
  }
  load.disabled = !canLoadNative(status);
  load.textContent = status.native?.paused ? "QQ 集成已暂停" : "加载二进制本地库";
  message.textContent = status.lastError || (status.ready ? "就绪" : "正在初始化");
}

async function update() {
  try {
    render(await client.status());
  } catch (error) {
    message.textContent = errorMessage(error);
  }
}

refresh.addEventListener("click", update);
load.addEventListener("click", async () => {
  load.disabled = true;
  message.textContent = "正在释放并加载本地库…";
  try {
    render(await client.loadNative());
    message.textContent = "本地库加载完成";
  } catch (error) {
    await update();
    message.textContent = errorMessage(error);
  }
});

try {
  client = createClient(window.go);
  await update();
} catch (error) {
  refresh.disabled = true;
  message.textContent = errorMessage(error);
}
