const REQ_KEY = "realman_avatar_picture_omni20_cv";
// BytePlus does not report duration. Use the configured policy of ten billable
// seconds per video; this is not a measurement or an upstream duration limit.
const BILLING_SECONDS = 10;

export const meta = {
  apiVersion: 1,
  key: "omnihuman",
  name: "OmniHuman 2.0",
  icon: "text:OH",
  version: "1.0.0",
  author: { name: "QuantumNous" },
  description: {
    en: "BytePlus OmniHuman video generation from an image and audio",
    zh: "BytePlus OmniHuman 图片与音频驱动视频生成",
  },
  baseUrl: "https://cv.byteplusapi.com",
  models: ["omnihuman-2.0", REQ_KEY],
  fetchMode: "per_task",
  upstreams: ["vendor", "new_api"],
  usageSchema: {
    seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } },
    // Preserve existing per-video expressions alongside per-second pricing.
    videos: {
      type: "number",
      unit: "count",
      unitLabel: { en: "video", zh: "个" },
      description: { en: "Video generation unit price", zh: "视频生成单价" },
    },
  },
  usageExamples: [{ label: "OmniHuman 2.0 · 10s", facts: { seconds: BILLING_SECONDS, videos: 1 } }],
  routes: [{ method: "POST", path: "/omnihuman/", type: "dynamic", decode: "decodeRequest", render: "renderTask" }],
  protocols: ["openai_video", { name: "openai_responses", supports: ["stream", "sync", "background"] }],
};

function object(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function mediaURL(value, field) {
  if (typeof value !== "string" || !/^https?:\/\/[^\s/?#@]+(?:[/?#][^\s]*)?$/i.test(value)) {
    throw new Error(field + " must be an HTTP or HTTPS URL");
  }
  return value;
}

function submitIntent(model, req) {
  if (typeof model !== "string" || !model.trim()) throw new Error("model is required");
  if (req.metadata !== undefined && !object(req.metadata)) throw new Error("metadata must be an object");
  const metadata = req.metadata || {};
  const body = {};
  for (const key of ["prompt", "audio_url", "image_urls", "enable_watermark"]) {
    if (req[key] !== undefined) body[key] = req[key];
    else if (metadata[key] !== undefined) body[key] = metadata[key];
  }
  if (body.image_urls === undefined && req.input_reference !== undefined) body.image_urls = [req.input_reference];
  if (body.image_urls === undefined && req.image !== undefined) body.image_urls = [req.image];
  if (!Array.isArray(body.image_urls) || body.image_urls.length !== 1) throw new Error("image_urls must contain exactly one image URL");
  body.image_urls = body.image_urls.map(function (url) {
    return mediaURL(url, "image_urls");
  });
  body.audio_url = mediaURL(body.audio_url, "audio_url");
  if (body.prompt !== undefined && typeof body.prompt !== "string") throw new Error("prompt must be a string");
  if (body.enable_watermark !== undefined && typeof body.enable_watermark !== "boolean") throw new Error("enable_watermark must be a boolean");
  return { kind: "submit", model: model, action: "image_to_video", requestBody: body };
}

function upstreamRequest(ctx, action, payload) {
  const gateway = ctx.upstream && ctx.upstream.kind === "new_api";
  const url = ctx.baseUrl.replace(/\/+$/, "") + (gateway ? "/omnihuman/" : "/") + "?Action=" + action + "&Version=2024-06-06";
  const body = JSON.stringify(payload);
  const headers = { "Content-Type": "application/json", Accept: "application/json" };
  if (gateway) {
    headers.Authorization = ctx.authHeader;
  } else {
    if (!url.startsWith("https://")) throw new Error("OmniHuman requires an HTTPS upstream URL");
    const parts = String(ctx.apiKey || "").split("|");
    if (parts.length !== 2 || !parts[0].trim() || !parts[1].trim()) throw new Error("OmniHuman credentials must use ak|sk format");
    Object.assign(
      headers,
      utils.volcSignV4({
        Method: "POST",
        URL: url,
        Headers: { "Content-Type": "application/json" },
        Body: body,
        AccessKey: parts[0].trim(),
        SecretKey: parts[1].trim(),
        Region: "ap-singapore-1",
        Service: "cv",
      }),
    );
  }
  return { url: url, method: "POST", headers: headers, body: body };
}

export function buildSubmitRequest(ctx) {
  // Revalidate the normalized body after channel selection. Only documented
  // vendor fields are forwarded; metadata cannot override the service key.
  const body = submitIntent(ctx.model, ctx.requestBody || {}).requestBody;
  // Another gateway routes on the configured model identity before its own
  // driver converts that name to the vendor's fixed service key.
  const reqKey = ctx.upstream && ctx.upstream.kind === "new_api" ? ctx.upstreamModel || ctx.model : REQ_KEY;
  return upstreamRequest(ctx, "CVSync2AsyncSubmitTask", Object.assign({ req_key: reqKey }, body));
}

export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  if (body.code !== 10000) throw new Error(body.message || "OmniHuman submission failed");
  if (!body.data || typeof body.data.task_id !== "string" || !body.data.task_id.trim()) throw new Error("OmniHuman response is missing task_id");
  return { taskId: body.data.task_id, taskData: body };
}

export function buildQueryRequest(ctx) {
  return upstreamRequest(ctx, "CVSync2AsyncGetResult", { req_key: REQ_KEY, task_id: ctx.taskId });
}

export function parseTaskResult(ctx, body) {
  if (!object(body) || typeof body.code !== "number") return { status: "UNKNOWN", reason: "Invalid OmniHuman response" };
  // Retry transient application errors even when the provider returns HTTP 200.
  if ([50429, 50430, 50500, 50501].includes(body.code)) return { status: "UNKNOWN", reason: body.message || "Temporary OmniHuman error" };
  if (body.code !== 10000) return { status: "FAILURE", progress: "100%", code: body.code, reason: body.message || "OmniHuman task failed" };
  const data = body.data || {};
  if (data.status === "in_queue") return { status: "QUEUED", progress: "10%" };
  if (data.status === "generating") return { status: "IN_PROGRESS", progress: "50%" };
  if (data.status === "done") {
    if (typeof data.video_url !== "string" || !data.video_url.trim()) return { status: "UNKNOWN", reason: "OmniHuman completed without a video URL" };
    return { status: "SUCCESS", progress: "100%", url: mediaURL(data.video_url, "video_url") };
  }
  if (["not_found", "expired", "canceled"].includes(data.status)) return { status: "FAILURE", progress: "100%", reason: "OmniHuman task " + data.status };
  return { status: "UNKNOWN", reason: "Unrecognized OmniHuman task status: " + String(data.status || "") };
}

export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return null;
  return { seconds: BILLING_SECONDS, videos: 1 };
}

export function extractUsageOnComplete(ctx, result) {
  return result.status === "SUCCESS" ? { seconds: BILLING_SECONDS, videos: 1 } : null;
}

export function listArtifacts(task) {
  const data = (task.data || {}).data || {};
  return task.status === "SUCCESS" && data.video_url ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

export function buildContentRequest(ctx) {
  const data = (ctx.data || {}).data || {};
  if (ctx.artifactKey !== "video" || !data.video_url) throw new Error("artifact_not_found");
  return { url: mediaURL(data.video_url, "video_url"), method: ctx.clientRequest.method, credentialless: true };
}

export const native = {
  decodeRequest: function (ctx) {
    if (!ctx.body || ctx.body.kind !== "json" || !object(ctx.body.value)) throw new Error("JSON object required");
    const actions = (ctx.query || {}).Action || [];
    if (actions.length !== 1) throw new Error("Exactly one Action query parameter is required");
    const body = ctx.body.value;
    if (actions[0] === "CVSync2AsyncGetResult") {
      if (typeof body.task_id !== "string" || !body.task_id.trim()) throw new Error("task_id is required");
      return { kind: "query", taskIds: [body.task_id] };
    }
    if (actions[0] !== "CVSync2AsyncSubmitTask") throw new Error("Unsupported OmniHuman Action; use submit or query");
    return submitIntent(body.req_key, body);
  },
  renderTask: function (ctx, tasks) {
    const task = Array.isArray(tasks) ? tasks[0] : tasks;
    const stored = task.data || {};
    const data = Object.assign({}, stored.data || {}, { task_id: task.task_id });
    const statuses = { NOT_START: "in_queue", SUBMITTED: "in_queue", QUEUED: "in_queue", IN_PROGRESS: "generating", SUCCESS: "done", FAILURE: "done" };
    data.status = statuses[task.status] || data.status;
    const failed = task.status === "FAILURE";
    // A host-terminal failure must remain terminal when another gateway polls
    // us, even if the last provider snapshot contained a retryable error.
    const code = failed ? (stored.code && stored.code !== 10000 && ![50429, 50430, 50500, 50501].includes(stored.code) ? stored.code : 500) : 10000;
    return Object.assign({}, stored, {
      code: code,
      status: code,
      message: failed ? task.fail_reason || stored.message || "OmniHuman task failed" : "Success",
      data: data,
    });
  },
  error: function (ctx, error) {
    return { code: error.httpStatus, message: error.message, data: null };
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
      if (ctx.body.kind === "json" && object(ctx.body.value)) req = ctx.body.value;
      else if (ctx.body.kind === "multipart") {
        if ((ctx.body.files || []).length) throw new Error("OmniHuman requires image and audio URLs; file uploads are unsupported");
        req = {};
        for (const name of Object.keys(ctx.body.fields || {})) {
          const values = ctx.body.fields[name];
          if (values.length !== 1) throw new Error(name + " must be provided once");
          req[name] = values[0];
        }
        for (const name of ["metadata", "image_urls", "enable_watermark"]) {
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
      const output = Object.assign({}, task.data || {});
      const data = output.data || {};
      if (data.video_url) output.url = data.video_url;
      if (task.status === "FAILURE") output.error = { code: "task_failed", message: task.fail_reason || "OmniHuman task failed" };
      return output;
    },
  },
  openai_responses: {
    decodeRequest: function (ctx) {
      if (!ctx.body || ctx.body.kind !== "json" || !object(ctx.body.value)) throw new Error("JSON object required");
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
          if (!object(item)) throw new Error("Invalid Responses input item");
          const content = item.content === undefined ? [item] : Array.isArray(item.content) ? item.content : [item.content];
          for (const part of content) {
            if (typeof part === "string") {
              texts.push(part);
              continue;
            }
            if (!object(part)) throw new Error("Invalid Responses content");
            if (["input_text", "text"].includes(part.type) && typeof part.text === "string") texts.push(part.text);
            else if (part.type === "input_image") images.push(part.image_url);
            else throw new Error("OmniHuman Responses accepts text and image input; provide audio_url in metadata");
          }
        }
      if (texts.length) req.prompt = texts.join("\n");
      if (images.length) req.image_urls = images;
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
        return { events: [{ type: "error", code: "task_failed", message: task.fail_reason || "OmniHuman task failed" }], state: state, done: true };
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
        metadata: { vendor: "omnihuman" },
      };
    },
  },
};
