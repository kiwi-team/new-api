const OMNIHUMAN_MODEL = "bytedance/omnihuman/v2";
const OMNIHUMAN_APP = "/bytedance/omnihuman";
const MAX_SECONDS = 300;

export const meta = {
  apiVersion: 1,
  key: "fal",
  name: "fal",
  icon: "text:fal",
  version: "1.0.0",
  author: { name: "QuantumNous" },
  description: { en: "OmniHuman 2.0 video generation through fal", zh: "通过 fal 生成 OmniHuman 2.0 视频" },
  baseUrl: "https://queue.fal.run",
  models: ["omnihuman-2.0", OMNIHUMAN_MODEL],
  fetchMode: "per_task",
  upstreams: ["vendor", "new_api"],
  usageSchema: {
    seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } },
  },
  usageExamples: [{ label: "OmniHuman 2.0 · 10s", facts: { seconds: 10 } }],
  routes: [
    { method: "POST", path: "/fal/bytedance/omnihuman/v2", type: "submit", decode: "createTask", render: "taskCreated" },
    { method: "GET", path: "/fal/bytedance/omnihuman/requests/:task_id/status", type: "query", render: "taskStatus" },
    { method: "GET", path: "/fal/bytedance/omnihuman/requests/:task_id", type: "query", render: "taskResult" },
  ],
  protocols: ["openai_video", { name: "openai_responses", supports: ["stream", "sync", "background"] }],
};

function isObject(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function mediaURL(value, field) {
  if (typeof value !== "string" || !/^https?:\/\/[^\s/?#@]+(?:[/?#][^\s]*)?$/i.test(value)) throw new Error(field + " must be an HTTP or HTTPS URL");
  return value;
}

function submitIntent(model, req) {
  if (typeof model !== "string" || !model.trim()) throw new Error("model is required");
  if (req.metadata !== undefined && !isObject(req.metadata)) throw new Error("metadata must be an object");
  const metadata = req.metadata || {};
  const body = {};
  // image_urls, input_reference and image keep the unified video input compatible
  // with the BytePlus plugin. fal itself receives a single image_url.
  for (const key of ["image_url", "image_urls", "input_reference", "audio_url", "prompt"]) {
    if (req[key] !== undefined) body[key] = req[key];
    else if (metadata[key] !== undefined) body[key] = metadata[key];
  }
  if (body.image_urls !== undefined) {
    if (!Array.isArray(body.image_urls) || body.image_urls.length !== 1) throw new Error("image_urls must contain exactly one image URL");
    if (body.image_url === undefined) body.image_url = body.image_urls[0];
  }
  if (body.image_url === undefined) body.image_url = body.input_reference;
  if (body.image_url === undefined) body.image_url = req.image;
  delete body.image_urls;
  delete body.input_reference;
  body.image_url = mediaURL(body.image_url, "image_url");
  body.audio_url = mediaURL(body.audio_url, "audio_url");
  if (body.prompt !== undefined && body.prompt !== null && typeof body.prompt !== "string") throw new Error("prompt must be a string or null");
  return { kind: "submit", model: model, action: "image_to_video", requestBody: body };
}

function upstreamRequest(ctx, path, body) {
  const gateway = ctx.upstream && ctx.upstream.kind === "new_api";
  const base = ctx.baseUrl.replace(/\/+$/, "");
  const headers = { Accept: "application/json" };
  if (gateway) headers.Authorization = ctx.authHeader;
  else {
    if (!base.startsWith("https://")) throw new Error("fal requires an HTTPS upstream URL");
    const key = String(ctx.apiKey || "").trim();
    if (!key || /[\r\n]/.test(key)) throw new Error("A valid fal API key is required");
    headers.Authorization = "Key " + key;
  }
  const request = { url: base + (gateway ? "/fal" : "") + path, method: "GET", headers: headers };
  if (body !== undefined) {
    request.method = "POST";
    request.headers["Content-Type"] = "application/json";
    request.body = body;
  }
  return request;
}

export function buildSubmitRequest(ctx) {
  const body = submitIntent(ctx.model, ctx.requestBody || {}).requestBody;
  if (ctx.upstream && ctx.upstream.kind === "new_api") body.model = ctx.upstreamModel || ctx.model;
  return upstreamRequest(ctx, "/" + OMNIHUMAN_MODEL, body);
}

export function parseSubmitResponse(ctx, response) {
  const body = response.body;
  if (!isObject(body) || typeof body.request_id !== "string" || !body.request_id.trim()) throw new Error("fal response is missing request_id");
  return { taskId: body.request_id, taskData: body };
}

export function buildQueryRequest(ctx) {
  // Queue lookup uses owner/app, without the /v2 endpoint suffix. Rebuild
  // these URLs so returned status_url/response_url cannot redirect credentials.
  const path = OMNIHUMAN_APP + "/requests/" + encodeURIComponent(ctx.taskId);
  return upstreamRequest(ctx, path + (ctx.state && ctx.state.phase === "result" ? "" : "/status"));
}

export function parseTaskResult(ctx, body, response) {
  if (!isObject(body)) return { status: "UNKNOWN", reason: "Invalid fal response" };
  if (response && response.status >= 400) {
    if ([401, 403, 429].includes(response.status) || response.status >= 500) return { status: "UNKNOWN", reason: "Temporary fal query error" };
    return { status: "FAILURE", progress: "100%", reason: typeof body.detail === "string" ? body.detail : "fal rejected the task" };
  }
  if (body.error || body.error_type) return { status: "FAILURE", progress: "100%", reason: typeof body.error === "string" ? body.error : "fal task failed" };
  if (body.status === "IN_QUEUE") return { status: "QUEUED", progress: "10%" };
  if (body.status === "IN_PROGRESS") return { status: "IN_PROGRESS", progress: "50%" };
  if (body.status === "COMPLETED" && !(ctx.state && ctx.state.phase === "result")) {
    // COMPLETED describes the queue, not a successful model result. Persist
    // this phase so the next poll (including after restart) fetches the output.
    return { status: "IN_PROGRESS", progress: "90%", state: { phase: "result" } };
  }
  if (ctx.state && ctx.state.phase === "result" && isObject(body.video) && typeof body.video.url === "string" && body.video.url) {
    return { status: "SUCCESS", progress: "100%", url: mediaURL(body.video.url, "video.url") };
  }
  return { status: "UNKNOWN", reason: "Unrecognized fal task result" };
}

export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return null;
  // The input is an external audio URL: client duration claims are not a
  // trustworthy meter. Reserve the documented maximum and settle below.
  return { seconds: MAX_SECONDS };
}

export function extractUsageOnComplete(ctx, result, body) {
  if (result.status !== "SUCCESS") return null;
  const seconds = body && body.duration;
  // Missing/invalid usage keeps the reservation; metrics.inference_time is
  // compute time, not the audio/video duration used by fal for billing.
  if (typeof seconds !== "number" || !Number.isFinite(seconds) || seconds < 4 || seconds > MAX_SECONDS) return null;
  return { seconds: seconds };
}

export function listArtifacts(task) {
  const video = (task.data || {}).video;
  return task.status === "SUCCESS" && video && video.url ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

export function buildContentRequest(ctx) {
  const video = (ctx.data || {}).video;
  if (ctx.artifactKey !== "video" || !video || !video.url) throw new Error("artifact_not_found");
  return { url: mediaURL(video.url, "video.url"), method: ctx.clientRequest.method, credentialless: true };
}

function nativeStatus(task) {
  const statuses = { NOT_START: "IN_QUEUE", SUBMITTED: "IN_QUEUE", QUEUED: "IN_QUEUE", IN_PROGRESS: "IN_PROGRESS", SUCCESS: "COMPLETED", FAILURE: "COMPLETED" };
  const path = "/fal" + OMNIHUMAN_APP + "/requests/" + encodeURIComponent(task.task_id);
  const output = { request_id: task.task_id, status: statuses[task.status] || "IN_PROGRESS", response_url: path, status_url: path + "/status" };
  if (task.status === "FAILURE") output.error = task.fail_reason || "fal task failed";
  return output;
}

export const native = {
  createTask: function (ctx) {
    if (!ctx.body || ctx.body.kind !== "json" || !isObject(ctx.body.value)) throw new Error("JSON object required");
    const body = ctx.body.value;
    return submitIntent(body.model === undefined ? OMNIHUMAN_MODEL : body.model, body);
  },
  taskCreated: function (ctx, task) {
    return nativeStatus(task);
  },
  taskStatus: function (ctx, task) {
    return nativeStatus(task);
  },
  taskResult: function (ctx, task) {
    if (task.status !== "SUCCESS") return nativeStatus(task);
    const data = task.data || {};
    return { video: data.video, duration: data.duration };
  },
  error: function (ctx, error) {
    return { detail: error.message };
  },
};

function responsesVideoText(ctx) {
  const artifact = ctx.artifacts && ctx.artifacts.video;
  if (!artifact || !artifact.url) throw new Error("video artifact is unavailable");
  const url = artifact.url.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return '<video controls src="' + url + '"></video>';
}

export const protocols = {
  openai_video: {
    decodeRequest: function (ctx) {
      if (!ctx.body) throw new Error("JSON or multipart body required");
      let req;
      if (ctx.body.kind === "json" && isObject(ctx.body.value)) req = ctx.body.value;
      else if (ctx.body.kind === "multipart") {
        if ((ctx.body.files || []).length) throw new Error("fal OmniHuman requires image and audio URLs; file uploads are unsupported");
        req = {};
        for (const name of Object.keys(ctx.body.fields || {})) {
          const values = ctx.body.fields[name];
          if (values.length !== 1) throw new Error(name + " must be provided once");
          req[name] = values[0];
        }
        for (const name of ["metadata", "image_urls"]) {
          if (req[name] !== undefined) {
            try {
              req[name] = JSON.parse(req[name]);
            } catch (e) {
              throw new Error(name + " must contain valid JSON");
            }
          }
        }
      } else throw new Error("JSON or multipart object required");
      return submitIntent(ctx.model, req);
    },
    render: function (ctx, task) {
      const data = task.data || {};
      const output = {};
      if (task.status === "SUCCESS") {
        output.video = data.video;
        output.duration = data.duration;
        output.url = data.video && data.video.url;
      }
      if (task.status === "FAILURE") output.error = { code: "task_failed", message: task.fail_reason || "fal task failed" };
      return output;
    },
  },
  openai_responses: {
    decodeRequest: function (ctx) {
      if (!ctx.body || ctx.body.kind !== "json" || !isObject(ctx.body.value)) throw new Error("JSON object required");
      const req = Object.assign({}, ctx.body.value);
      if (req.input !== undefined && typeof req.input !== "string" && !Array.isArray(req.input)) throw new Error("input must be a string or array");
      const texts = [];
      const images = [];
      if (typeof req.input === "string") texts.push(req.input);
      else
        for (const item of req.input || []) {
          if (typeof item === "string") {
            texts.push(item);
            continue;
          }
          if (!isObject(item)) throw new Error("Invalid Responses input item");
          const content = item.content === undefined ? [item] : Array.isArray(item.content) ? item.content : [item.content];
          for (const part of content) {
            if (typeof part === "string") {
              texts.push(part);
              continue;
            }
            if (!isObject(part)) throw new Error("Invalid Responses content");
            if (["input_text", "text"].includes(part.type) && typeof part.text === "string") texts.push(part.text);
            else if (part.type === "input_image") images.push(part.image_url);
            else throw new Error("fal OmniHuman Responses accepts text and image input; provide audio_url in metadata");
          }
        }
      if (texts.length) req.prompt = texts.join("\n");
      if (images.length) {
        if (images.length !== 1) throw new Error("Exactly one input image is required");
        req.image_url = images[0];
      }
      return submitIntent(ctx.model, req);
    },
    renderEvents: function (ctx, task, previousState) {
      const status = task.status || "UNKNOWN";
      const progress = Number(String(task.progress || "0").replace("%", ""));
      const state = { status: status, progress: progress };
      if (status === "SUCCESS")
        return {
          events: previousState && previousState.status === status ? [] : [{ type: "output", data: responsesVideoText(ctx) }],
          state: state,
          done: true,
        };
      if (status === "FAILURE")
        return { events: [{ type: "error", code: "task_failed", message: task.fail_reason || "fal task failed" }], state: state, done: true };
      if (previousState && previousState.status === status && previousState.progress === progress) return { events: [], state: state, done: false };
      return { events: [{ type: "progress", message: status.toLowerCase(), progress: progress }], state: state, done: false };
    },
    renderFinal: function (ctx, task) {
      return {
        output: [
          {
            type: "message",
            status: "completed",
            role: "assistant",
            content: [{ type: "output_text", text: responsesVideoText(ctx), annotations: [], logprobs: [] }],
          },
        ],
        metadata: { vendor: "fal" },
      };
    },
  },
};
