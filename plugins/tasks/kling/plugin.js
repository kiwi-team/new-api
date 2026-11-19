export const meta = {
  apiVersion: 1,
  key: "kling",
  name: "Kling",
  icon: "Kling.Color",
  description: {
    en: "Kuaishou Kling video generation with text, keyframes and multimodal references",
    zh: "快手可灵视频生成，支持文本、关键帧和多模态参考",
  },
  version: "1.3.0",
  requiredCapabilities: ["billing-pending@1"],
  author: { name: "QuantumNous" },
  channelTypes: [50],
  models: ["kling-v1", "kling-v1-6", "kling-v2-master", "kling-v2-5-turbo", "kling-v2-6", "kling-v3", "kling-4.0"],
  fetchMode: "per_task",
  upstreams: ["vendor", "new_api"],
  usageSchema: {
    // Kling final unit deduction (estimated at submit, actual on completion).
    units: {
      type: "number",
      unit: "credit",
      description: { en: "Kling credit unit price", zh: "可灵资源包单位单价" },
    },
  },
  usageProfiles: [{
    models: ["kling-4.0"],
    schema: {
      billable_seconds: { type: "number", unit: "second", description: { en: "Video generation unit price", zh: "视频生成单价" } },
      resolution: { enum: ["720p", "1080p"], description: { en: "Output video resolution", zh: "输出视频分辨率" } },
      video_input: {
        enum: ["none", "video"], description: { en: "Reference video input", zh: "参考视频输入" },
        enumLabels: { none: { en: "No input video", zh: "无输入视频" }, video: { en: "With input video", zh: "有输入视频" } },
      },
    },
    examples: [
      { label: "720p · 10s", facts: { billable_seconds: 10, resolution: "720p", video_input: "none" } },
      { label: "1080p · 10s", facts: { billable_seconds: 10, resolution: "1080p", video_input: "none" } },
      { label: "1080p · input 8.03s · output 5s", facts: { billable_seconds: 13.03, resolution: "1080p", video_input: "video" } },
    ],
  }],
  usageExamples: [
    { label: "v1 std 5s", facts: { units: 1 } },
    { label: "v1 pro 5s", facts: { units: 3.5 } },
    { label: "v1-6 std 5s", facts: { units: 2 } },
    { label: "v1-6 pro 10s", facts: { units: 7 } },
    { label: "v2-master pro 5s", facts: { units: 10 } },
    { label: "v3 pro audio 5s", facts: { units: 6 } },
    { label: "v3 4k audio 5s", facts: { units: 15 } },
    { label: "v2.5 turbo std 5s", facts: { units: 1.5 } },
    { label: "v2.6 pro 10s · audio", facts: { units: 10 } },
    { label: "v3 pro 5s", facts: { units: 4 } },
    { label: "v3 4k 5s", facts: { units: 15 } },
  ],
  protocols: [{ name: "openai_responses", supports: ["stream", "sync", "background"], models: ["kling-v1", "kling-v1-6", "kling-v2-master", "kling-v2-5-turbo", "kling-v2-6", "kling-v3"] }, "openai_video"],
  // Kling's own paths; the gateway also serves each under /kling.
  routes: [
    { method: "POST", path: "/v1/videos/text2video", type: "submit", action: "text_to_video", decode: "decodeSubmit", render: "taskCreated" },
    { method: "POST", path: "/v1/videos/image2video", type: "submit", action: "image_to_video", decode: "decodeSubmit", render: "taskCreated" },
    { method: "GET", path: "/v1/videos/text2video/:task_id", type: "query", render: "taskStatus" },
    { method: "GET", path: "/v1/videos/image2video/:task_id", type: "query", render: "taskStatus" },
    // Kling 4.0 takes the model in the path and answers its tasks at
    // GET /tasks?task_ids=.
    { method: "POST", path: "/text-to-video/kling-4.0", type: "submit", action: "text_to_video", decode: "kling4TextToVideo", render: "kling4Created" },
    { method: "POST", path: "/image-to-video/kling-4.0", type: "submit", action: "image_to_video", decode: "kling4ImageToVideo", render: "kling4Created" },
    { method: "POST", path: "/omni-video/kling-4.0", type: "submit", action: "reference_to_video", decode: "kling4OmniVideo", render: "kling4Created" },
    { method: "GET", path: "/tasks", type: "query", taskIdParam: "task_ids", render: "kling4Tasks" },
  ],
};

// The official resource-package consumption of Kling's models in units per
// output second of each mode, not a currency price
// (kling.ai/document-api/pricing/base/video): silent, with native audio (sound
// "on"), and with a reference voice (voice_list). Kling checks modes and
// durations itself and reports the units it deducted as final_unit_deduction,
// which settles the task.
const KLING_MODELS = {
  "kling-v2-5-turbo": { units: { std: 0.3, pro: 0.5 } },
  "kling-v2-6": { units: { std: 0.3, pro: 0.5 }, soundUnits: { pro: 1.0 }, voiceUnits: { pro: 1.2 } },
  "kling-v3": { units: { std: 0.6, pro: 0.8, "4k": 3.0 }, soundUnits: { std: 0.9, pro: 1.2, "4k": 3.0 } },
  "kling-v1": { units: { std: 0.2, pro: 0.7 } },
  "kling-v1-6": { units: { std: 0.4, pro: 0.7 } },
  "kling-v2-master": { units: { pro: 2 } },
};

// A channel may map a declared model to a name outside the table; it reserves
// the highest consumption of each mode and settles on the units Kling reports.
const DEFAULT_KLING_MODEL = {
  units: { std: 0.9, pro: 1.2, "4k": 3.0 },
  soundUnits: { std: 0.9, pro: 1.2, "4k": 3.0 },
  voiceUnits: { std: 1.2, pro: 1.2, "4k": 3.0 },
};

// Kling 4.0 takes the model in the path and bills units per second of a total
// duration (kling.ai/document-api/pricing/base/video): the output alone, or,
// with an input video, twice the output, or input plus output when the input
// is longer. Input videos are feature_video, base_video and video character
// elements; Kling reports their length in usage.input_duration.
const KLING4_UNITS = { "720p": 1.0, "1080p": 1.5 };
const KLING4_VIDEO_INPUT_UNITS = { "720p": 0.6, "1080p": 0.9 };
// The shortest and the longest output in seconds; the longest is also the
// longest total of input videos.
const KLING4_MIN_SECONDS = 3;
const KLING4_MAX_SECONDS = 30;
// Whether an element is a video character is not in the request, so an
// element counts as an input video.
const KLING4_ACTIONS = { "text-to-video": "text_to_video", "image-to-video": "image_to_video", "omni-video": "reference_to_video" };
// The text-to-video aspect ratios of kling-v2-5-turbo, kling-v2-6 and
// kling-v3, then of Kling 4.0. The host protocols send the one nearest the
// request's size; without a size Kling uses its default, 16:9.
const KLING_ASPECT_RATIOS = ["16:9", "9:16", "1:1"];

// Kling's numeric error codes for the statuses the gateway answers with
// (kling.ai/document-api/api/get-started/error-codes).
const ERROR_CODES = { 400: 1201, 401: 1000, 403: 1103, 404: 1203, 429: 1302, 500: 5000, 503: 5001, 504: 5002 };

// Kling would call these back with its own task ids, which the gateway does
// not hand out, so a native request carrying them is refused.
const UNSUPPORTED_FIELDS = ["callback_url", "external_task_id"];

function trimmed(value) {
  return String(value || "").trim();
}

function responsesInput(req) {
  const texts = [],
    images = [];
  const input = req.input;
  if (typeof input === "string") texts.push(input);
  else if (Array.isArray(input)) {
    for (const item of input) {
      if (typeof item === "string") {
        texts.push(item);
        continue;
      }
      if (!item || typeof item !== "object" || Array.isArray(item)) continue;
      const content = item.content === undefined ? [item] : Array.isArray(item.content) ? item.content : [item.content];
      for (const part of content) {
        if (typeof part === "string") {
          texts.push(part);
          continue;
        }
        if (!part || typeof part !== "object" || Array.isArray(part)) continue;
        if (["input_text", "text"].includes(part.type) && typeof part.text === "string") texts.push(part.text);
        if (["input_image", "image_url"].includes(part.type)) {
          let image = part.image_url;
          if (image && typeof image === "object") image = image.url;
          if (trimmed(image)) images.push(trimmed(image));
        }
      }
    }
  }
  return {
    prompt: texts
      .filter(function (text) {
        return trimmed(text);
      })
      .join("\n"),
    images: images,
  };
}

function responsesVideoText(ctx) {
  const artifact = ctx && ctx.artifacts && ctx.artifacts.video;
  const url = trimmed(artifact && artifact.url);
  if (!url) throw new Error("video artifact is unavailable");
  const escaped = url.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return '<video controls src="' + escaped + '"></video>';
}

function isRelay(apiKey) {
  return apiKey.startsWith("sk-");
}

// The host signal is authoritative on New API channels; the sk- key prefix
// stays as the heuristic for legacy type-50 channels pointed at a gateway.
function viaGateway(ctx) {
  return !!(ctx.upstream && ctx.upstream.kind === "new_api") || isRelay(ctx.apiKey);
}

function tokenFor(ctx) {
  const apiKey = trimmed(ctx.apiKey);
  if (!apiKey) throw new Error("api_key is empty");
  if (viaGateway(ctx)) return apiKey;
  const parts = apiKey.split("|");
  if (parts.length === 1) return apiKey;
  if (parts.length !== 2) throw new Error("invalid api_key, required format is accessKey|secretKey");
  const now = utils.unixNow();
  return utils.jwtSignHS256({ iss: parts[0].trim(), exp: now + 1800, nbf: now - 5 }, parts[1].trim());
}

function pathFor(action) {
  return action === "image_to_video" ? "/v1/videos/image2video" : "/v1/videos/text2video";
}

function apiRoot(ctx) {
  return ctx.baseUrl + (viaGateway(ctx) ? "/kling" : "");
}

function urlFor(ctx, action) {
  return apiRoot(ctx) + pathFor(action);
}

function requestHeaders(ctx) {
  return { Accept: "application/json", Authorization: "Bearer " + tokenFor(ctx), "User-Agent": "kling-sdk/1.0" };
}

// A host-protocol size, WIDTHxHEIGHT.
function sizeDimensions(size) {
  const parts = trimmed(size).toLowerCase().split("x");
  const width = Number(parts[0]);
  const height = Number(parts[1]);
  if (parts.length !== 2 || !(width > 0) || !(height > 0)) throw new Error("size must be WIDTHxHEIGHT, such as 1280x720");
  return { width: width, height: height };
}

// The aspect ratio of the list nearest the size's.
function aspectRatio(dimensions, ratios) {
  let nearest = ratios[0];
  let distance = Infinity;
  for (const ratio of ratios) {
    const sides = ratio.split(":").map(Number);
    const ratioDistance = Math.abs(Math.log(dimensions.width / dimensions.height) - Math.log(sides[0] / sides[1]));
    if (ratioDistance < distance) {
      nearest = ratio;
      distance = ratioDistance;
    }
  }
  return nearest;
}

function submitModel(ctx, req) {
  return trimmed((ctx && ctx.upstreamModel) || (ctx && ctx.model) || (req && req.model));
}

function resolveKlingMode(model, mode) {
  const raw = trimmed(mode).toLowerCase();
  if (model === "kling-v2-master") {
    if (raw === "std") throw new Error("kling-v2-master does not support mode std");
    if (raw && raw !== "pro") throw new Error("mode must be pro");
    return "pro";
  }
  if (!raw) return "std";
  if (model === "kling-v3" && raw === "4k") return raw;
  if (raw !== "std" && raw !== "pro") throw new Error(model === "kling-v3" ? "mode must be std, pro or 4k" : "mode must be std or pro");
  return raw;
}

function klingModel(model) {
  return Object.prototype.hasOwnProperty.call(KLING_MODELS, model) ? KLING_MODELS[model] : DEFAULT_KLING_MODEL;
}

// mode defaults to std; Kling checks it for the model.
function klingMode(mode) {
  return trimmed(mode).toLowerCase() || "std";
}

// Kling takes duration as a string and checks it for the model; a duration is
// billed, so the plugin bounds it first.
function validateKlingDuration(value) {
  const n = typeof value === "string" && value.trim() !== "" ? Number(value) : value;
  if (!Number.isInteger(n) || n <= 0 || n > 3600) throw new Error("duration must be a positive integer of at most 3600");
  return n;
}

// Kling 4.0 makes 3 to 30 seconds, so another duration is refused before it is
// reserved.
function kling4Duration(value) {
  const n = typeof value === "string" && value.trim() !== "" ? Number(value) : value;
  if (!Number.isInteger(n) || n < KLING4_MIN_SECONDS || n > KLING4_MAX_SECONDS)
    throw new Error("duration must be an integer from " + KLING4_MIN_SECONDS + " to " + KLING4_MAX_SECONDS);
  return n;
}

// The duration Kling receives and the one billed: metadata.duration (the whole
// body on native routes) overrides the top-level duration, and Kling's default
// is 5.
function outboundDuration(req) {
  const metadata = (req && req.metadata) || {};
  let value = metadata.duration;
  if (value === undefined || value === null) value = req && req.duration;
  if (value === undefined || value === null) return 5;
  return validateKlingDuration(value);
}

function outboundMode(req) {
  const metadata = (req && req.metadata) || {};
  return klingMode((req && req.mode) || metadata.mode);
}

// Units per output second: native audio (sound "on", default "off") and a
// reference voice cost more. A mode the model's table does not list reserves
// the table's dearest, and audio the model has no table for the dearest audio
// of any model; the units Kling reports settle the task.
function unitsPerSecond(model, mode, metadata) {
  const profile = klingModel(model);
  let table = profile.units;
  if (trimmed(metadata.sound).toLowerCase() === "on") {
    const voice = Array.isArray(metadata.voice_list) && metadata.voice_list.length > 0;
    table = (voice && profile.voiceUnits) || profile.soundUnits || (voice ? DEFAULT_KLING_MODEL.voiceUnits : DEFAULT_KLING_MODEL.soundUnits);
  }
  if (Object.prototype.hasOwnProperty.call(table, mode)) return table[mode];
  return Object.keys(table).reduce((dearest, key) => Math.max(dearest, table[key]), 0);
}

function hasKlingImage(req, hasInputReferenceFile) {
  if (hasInputReferenceFile) return true;
  const metadata = (req && req.metadata) || {};
  if (req && req.image && typeof req.image === "object" && !Array.isArray(req.image) && req.image.__fileRef) return true;
  return Boolean(trimmed(req && req.input_reference) || trimmed(req && req.image) || metadata.image || metadata.image_tail);
}

function filePlaceholder(image) {
  if (!image || typeof image !== "object" || Array.isArray(image) || !image.__fileRef) return image;
  const placeholder = { __fileRef: image.__fileRef, encoding: image.encoding };
  if (image.mimeType) placeholder.mimeType = image.mimeType;
  if (image.maxBytes !== undefined && image.maxBytes !== null) placeholder.maxBytes = image.maxBytes;
  return placeholder;
}

function isKling4(ctx) {
  return !!(ctx && ctx.state && ctx.state.kling4) || String((ctx && (ctx.upstreamModel || ctx.model)) || "").toLowerCase() === "kling-4.0";
}

function kling4Token(ctx) {
  const key = trimmed(ctx.apiKey);
  if (!key || key.includes("|") || /[\r\n]/.test(key)) throw new Error("Kling 4.0 requires an API Key");
  return key;
}

// Read the supported compatibility fields without letting metadata override
// the quantity or resolution that was validated and reserved.
function kling4Parameter(req, name) {
  const metadata = req.metadata || {};
  const settings = metadata.settings || {};
  let selected;
  for (const source of [req, metadata, settings]) {
    if (!Object.prototype.hasOwnProperty.call(source, name)) continue;
    const value = source[name];
    if (selected !== undefined && String(value) !== String(selected)) throw new Error("conflicting " + name);
    selected = value;
  }
  return selected;
}

function decodeKling4Video(ctx) {
  if (!ctx.body || !["json", "multipart"].includes(ctx.body.kind)) throw new Error("JSON or multipart body required");
  let req;
  if (ctx.body.kind === "json") {
    if (!ctx.body.value || typeof ctx.body.value !== "object" || Array.isArray(ctx.body.value)) throw new Error("JSON object required");
    req = utils.json.clone(ctx.body.value);
  } else {
    req = {};
    for (const name of Object.keys(ctx.body.fields || {})) {
      const values = ctx.body.fields[name];
      if (values.length !== 1) throw new Error(name + " must be provided once");
      req[name] = values[0];
    }
    for (const name of ["metadata", "references", "images"]) {
      if (req[name] === undefined) continue;
      try { req[name] = JSON.parse(req[name]); } catch (_) { throw new Error(name + " must be valid JSON"); }
    }
    const files = ctx.body.files || [];
    if (files.length > 1) throw new Error("input_reference must be provided once");
    if (files.length) {
      const file = files[0];
      if (file.field !== "input_reference" || req.input_reference !== undefined || req.image !== undefined) throw new Error("conflicting input_reference");
      if (file.size > 30 * 1024 * 1024) throw new Error("input_reference exceeds 30 MB");
      if (!["image/jpeg", "image/png"].includes(file.mimeType)) throw new Error("input_reference must be JPEG or PNG");
      req.input_reference = { __fileRef: file.ref, encoding: "base64", maxBytes: 30 * 1024 * 1024 };
    }
  }
  req.model = ctx.model;
  const prepared = kling4Payload(req);
  return { kind: "submit", model: ctx.model, action: prepared.action, requestBody: req };
}

function kling4Payload(req) {
  const metadata = req.metadata === undefined ? {} : req.metadata;
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) throw new Error("metadata must be an object");
  if (metadata.settings !== undefined && (!metadata.settings || typeof metadata.settings !== "object" || Array.isArray(metadata.settings))) throw new Error("metadata.settings must be an object");
  const settings = {};
  let duration;
  for (const name of ["seconds", "duration"]) {
    const raw = kling4Parameter(req, name);
    if (raw === undefined) continue;
    if ((typeof raw !== "number" && typeof raw !== "string") || !/^\d+$/.test(String(raw))) throw new Error("seconds must be an integer between 3 and 30");
    const value = Number(raw);
    if (!Number.isInteger(value) || value < 3 || value > 30) throw new Error("seconds must be an integer between 3 and 30");
    if (duration !== undefined && duration !== value) throw new Error("conflicting duration");
    duration = value;
  }
  if (duration !== undefined) settings.duration = duration;
  let resolution = kling4Parameter(req, "resolution");
  let ratio = kling4Parameter(req, "aspect_ratio");
  const size = kling4Parameter(req, "size");
  if (size !== undefined) {
    const sizes = { "1280x720": ["720p", "16:9"], "720x1280": ["720p", "9:16"], "1920x1080": ["1080p", "16:9"], "1080x1920": ["1080p", "9:16"] };
    if (!sizes[size]) throw new Error("unsupported Kling 4.0 size");
    if (resolution !== undefined && resolution !== sizes[size][0]) throw new Error("conflicting resolution");
    if (ratio !== undefined && ratio !== sizes[size][1]) throw new Error("conflicting aspect_ratio");
    resolution = sizes[size][0];
    ratio = sizes[size][1];
  }
  const mode = kling4Parameter(req, "mode");
  if (mode !== undefined) {
    if (!["std", "pro"].includes(mode)) throw new Error("Kling 4.0 mode must be std or pro");
    const mapped = mode === "std" ? "720p" : "1080p";
    if (resolution !== undefined && resolution !== mapped) throw new Error("conflicting resolution");
    resolution = mapped;
  }
  settings.resolution = resolution === undefined ? "1080p" : resolution;
  if (!["720p", "1080p"].includes(settings.resolution)) throw new Error("resolution must be 720p or 1080p");
  if (ratio !== undefined) {
    if (!["21:9", "16:9", "9:16", "1:1", "adaptive"].includes(ratio)) throw new Error("unsupported aspect_ratio");
    settings.aspect_ratio = ratio;
  }
  const audio = kling4Parameter(req, "audio");
  if (audio !== undefined) {
    if (!["native", "original"].includes(audio)) throw new Error("audio must be native or original");
    settings.audio = audio;
  }
  if (kling4Parameter(req, "sound") !== undefined || req.generate_audio !== undefined) throw new Error("use audio native or original for Kling 4.0");
  for (const key of Object.keys(metadata.settings || {})) {
    if (!["duration", "resolution", "aspect_ratio", "audio"].includes(key)) throw new Error("unsupported Kling 4.0 setting: " + key);
  }
  const contents = [];
  const prompt = req.prompt === undefined ? "" : req.prompt;
  if (typeof prompt !== "string") throw new Error("prompt must be a string");
  if (prompt.trim()) contents.push({ type: "prompt", text: prompt });
  let references = req.references === undefined ? [] : req.references;
  if (!Array.isArray(references)) throw new Error("references must be an array");
  const images = req.images === undefined ? [] : req.images;
  if (!Array.isArray(images)) throw new Error("images must be an array");
  const image = req.input_reference === undefined ? req.image : req.input_reference;
  if (req.input_reference !== undefined && req.image !== undefined && req.input_reference !== req.image) throw new Error("conflicting image");
  if (references.length && (image !== undefined || images.length || metadata.image_tail !== undefined)) throw new Error("references cannot be combined with image fields");
  if (!references.length) {
    if (image !== undefined && images.length) throw new Error("conflicting images");
    const legacy = image === undefined ? images : [image];
    references = legacy.map(function (url, index) {
      return { type: "image", role: legacy.length > 2 ? "reference_image" : index === 0 ? "first_frame" : "last_frame", url: url, id: "image_" + (index + 1) };
    });
    if (metadata.image_tail !== undefined) references.push({ type: "image", role: "last_frame", url: metadata.image_tail, id: "image_tail" });
  }
  const roles = { first_frame: ["image", "first_frame"], middle_frame: ["image", "middle_frame"], last_frame: ["image", "last_frame"], reference_image: ["image", "refer_image"], reference_video: ["video", "feature_video"], base_video: ["video", "base_video"], element: ["element", "element"], voice: ["voice", "voice"] };
  for (const ref of references) {
    if (!ref || typeof ref !== "object" || Array.isArray(ref)) throw new Error("invalid reference");
    const role = roles[ref.role];
    if (!role || (ref.type !== undefined && ref.type !== role[0])) throw new Error("unsupported reference type or role");
    const item = { type: role[1] };
    if (ref.id !== undefined) {
      if (typeof ref.id !== "string" || !ref.id.trim()) throw new Error("reference id must be a non-empty string");
      item.id = ref.id;
    }
    if (ref.role === "element" || ref.role === "voice") {
      const key = ref.role === "element" ? "element_id" : "voice_id";
      if (typeof ref[key] !== "string" || !ref[key].trim() || !item.id) throw new Error(key + " and id are required");
      item[key] = ref[key];
    } else {
      const url = ref.url;
      if (url && typeof url === "object" && url.__fileRef) item.url = filePlaceholder(url);
      else if (typeof url === "string" && url.trim()) item.url = url.trim();
      else throw new Error("reference url is required");
      if (role[0] === "video" && !/^https?:\/\//i.test(item.url)) throw new Error("video reference must be an HTTP(S) URL");
    }
    contents.push(item);
  }
  if (metadata.contents !== undefined) {
    if (references.length || !Array.isArray(metadata.contents)) throw new Error("conflicting contents");
    for (const item of metadata.contents) {
      if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("invalid content");
      if (item.type === "prompt" && contents.some(function (entry) { return entry.type === "prompt"; })) throw new Error("conflicting prompt");
      contents.push(utils.json.clone(item));
    }
  }
  const counts = {}, ids = new Set();
  let videoInput = "none", lastImage = "", firstImage = "";
  let promptBudget = 0;
  for (const item of contents) {
    if (!["prompt", "first_frame", "middle_frame", "last_frame", "refer_image", "feature_video", "base_video", "element", "voice"].includes(item.type)) throw new Error("unsupported content type");
    counts[item.type] = (counts[item.type] || 0) + 1;
    if (item.id !== undefined) {
      if (typeof item.id !== "string" || !item.id.trim() || ids.has(item.id)) throw new Error("invalid or duplicate reference id");
      ids.add(item.id);
    }
    if (item.type === "prompt") {
      if (typeof item.text !== "string" || !item.text.trim()) throw new Error("prompt must be a non-empty string");
      for (const ch of item.text) promptBudget += /[A-Za-z\x00-\x7f]/.test(ch) ? 1 / 26000 : 1 / 11000;
      continue;
    }
    if (["element", "voice"].includes(item.type)) {
      const key = item.type === "element" ? "element_id" : "voice_id";
      if (typeof item[key] !== "string" || !item[key].trim() || !item.id) throw new Error(key + " and id are required");
    } else if (!item.url || (typeof item.url !== "string" && !(typeof item.url === "object" && item.url.__fileRef))) throw new Error("reference url is required");
    if (["feature_video", "base_video"].includes(item.type)) {
      if (typeof item.url !== "string" || !/^https?:\/\//i.test(item.url)) throw new Error("video reference must be an HTTP(S) URL");
      videoInput = "video";
    } else if (item.type === "element" && videoInput === "none") videoInput = "unknown";
    if (["first_frame", "middle_frame", "last_frame", "refer_image"].includes(item.type)) {
      if (!firstImage) firstImage = item.type;
      if (lastImage === "last_frame" || (item.type === "first_frame" && lastImage)) throw new Error("keyframes must be ordered first, middle, last");
      lastImage = item.type;
    }
  }
  if (promptBudget > 1 + 1e-12 || (counts.prompt || 0) > 1) throw new Error("prompt exceeds the Kling 4.0 character budget");
  const imageCount = (counts.first_frame || 0) + (counts.middle_frame || 0) + (counts.last_frame || 0) + (counts.refer_image || 0);
  const videoCount = (counts.feature_video || 0) + (counts.base_video || 0);
  if (imageCount > 10 || (counts.first_frame || 0) > 1 || (counts.last_frame || 0) > 1 || videoCount > 5 || (counts.base_video || 0) > 1 || (counts.element || 0) > 7 || (counts.voice || 0) > 3 || imageCount + videoCount + (counts.element || 0) + (counts.voice || 0) > 15) throw new Error("too many Kling 4.0 references");
  for (const type of ["middle_frame", "refer_image"]) {
    if ((counts[type] || 0) > 1 && contents.some(function (item) { return item.type === type && !item.id; })) throw new Error("multiple " + type + " references require ids");
  }
  if (audio === "original" && (!videoCount || counts.voice)) throw new Error("original audio requires a video and cannot use a voice reference");
  if (!contents.length) throw new Error("prompt or references are required");
  let action = "text_to_video";
  if (counts.refer_image || videoCount || counts.element || counts.voice || audio !== undefined || ratio === "adaptive" || (imageCount && ratio !== undefined)) action = "omni_video";
  else if (imageCount) action = "image_to_video";
  if (action === "text_to_video" && !counts.prompt) throw new Error("prompt is required");
  const body = action === "text_to_video" ? { prompt: contents[0].text, settings: settings } : { contents: contents, settings: settings };
  const options = {};
  const rawOptions = metadata.options || {};
  if (!rawOptions || typeof rawOptions !== "object" || Array.isArray(rawOptions)) throw new Error("options must be an object");
  for (const name of ["callback_url", "external_task_id"]) {
    const value = metadata[name] === undefined ? rawOptions[name] : metadata[name];
    if (metadata[name] !== undefined && rawOptions[name] !== undefined && metadata[name] !== rawOptions[name]) throw new Error("conflicting " + name);
    if (value !== undefined) {
      if (typeof value !== "string") throw new Error(name + " must be a string");
      options[name] = value;
    }
  }
  const watermark = metadata.watermark === undefined ? rawOptions.watermark_info && rawOptions.watermark_info.enabled : metadata.watermark;
  if (watermark !== undefined) {
    if (typeof watermark !== "boolean") throw new Error("watermark must be a boolean");
    if (rawOptions.watermark_info && rawOptions.watermark_info.enabled !== undefined && rawOptions.watermark_info.enabled !== watermark) throw new Error("conflicting watermark");
    options.watermark_info = { enabled: watermark };
  }
  if (Object.keys(options).length) body.options = options;
  const state = { kling4: true, resolution: settings.resolution, videoInput: videoInput };
  if (duration !== undefined) state.duration = duration;
  return { body: body, action: action, state: state };
}

function buildKling4Submit(ctx) {
  const prepared = kling4Payload(ctx.requestBody);
  const gateway = ctx.upstream && ctx.upstream.kind === "new_api";
  prepared.state.gateway = !!gateway;
  const paths = { text_to_video: "/text-to-video/kling-4.0", image_to_video: "/image-to-video/kling-4.0", omni_video: "/omni-video/kling-4.0" };
  return { url: ctx.baseUrl + (gateway ? "/v1/videos" : paths[prepared.action]), method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json", Authorization: "Bearer " + kling4Token(ctx) },
    body: gateway ? Object.assign({}, ctx.requestBody, { model: "kling-4.0" }) : prepared.body,
    action: prepared.action, state: prepared.state };
}

function kling4Task(ctx, body) {
  if (!body || typeof body !== "object") return null;
  if (body.object === "video") return !ctx.taskId || body.id === ctx.taskId ? body : null;
  if (body.code !== 0) return null;
  const tasks = Array.isArray(body.data) ? body.data : body.data && body.data.result;
  if (!Array.isArray(tasks)) return null;
  for (const task of tasks) if (task && task.id === ctx.taskId) return task;
  return null;
}

function kling4Usage(ctx, body) {
  const task = kling4Task(ctx, body);
  if (!task || !["succeeded", "completed"].includes(task.status)) return null;
  const usage = task.usage || {};
  const inputRaw = usage.input_duration, outputRaw = usage.output_duration;
  if (inputRaw === undefined || inputRaw === null || inputRaw === "" || outputRaw === undefined || outputRaw === null || outputRaw === "") return null;
  if (!["number", "string"].includes(typeof inputRaw) || !["number", "string"].includes(typeof outputRaw)) return null;
  if (!/^\d+(\.\d+)?$/.test(String(inputRaw)) || !/^\d+(\.\d+)?$/.test(String(outputRaw))) return null;
  const input = Number(inputRaw), output = Number(outputRaw);
  if (!Number.isFinite(input) || !Number.isFinite(output) || input < 0 || input > 30 || output < 0 || output > 30 || Math.round(input * 100) / 100 !== input || !Number.isInteger(output)) return null;
  const state = ctx.state || {};
  if (state.videoInput === "video" && input === 0 && output !== 0) return null;
  const resolution = state.resolution || usage.resolution;
  if (!["720p", "1080p"].includes(resolution)) return null;
  // Input is reported to hundredths of a second; calculate in hundredths to
  // retain the published decimal quantity through task-usage normalization.
  const seconds = input === 0 ? output : (Math.round(Math.max(input, output) * 100) + Math.round(output * 100)) / 100;
  // Retain the measured components as audit facts; only billable_seconds is
  // exposed as the pricing quantity in the model profile.
  return { billable_seconds: seconds, resolution: resolution, video_input: input > 0 ? "video" : "none", input_seconds: input, output_seconds: output };
}


function plainObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

// 1080p is Kling's default and the dearer; a resolution Kling does not list
// reserves it.
function kling4Resolution(settings) {
  return trimmed(settings.resolution).toLowerCase() === "720p" ? "720p" : "1080p";
}

// Kling would call back, or look tasks up, by ids the gateway does not hand
// out. The duration is billed, so it is kept within Kling 4.0's range.
function checkKling4Body(body) {
  const options = plainObject(body.options);
  for (const name of UNSUPPORTED_FIELDS) {
    const value = options[name];
    if (value !== undefined && value !== null && value !== "") throw new Error("options." + name + " is not supported; query the task instead");
  }
  const settings = plainObject(body.settings);
  if (settings.duration !== undefined && settings.duration !== null) kling4Duration(settings.duration);
}

// Save the billing dimensions with the task. An element may represent a video
// character, so reserve it as a video until Kling reports its input duration.
function nativeKling4State(body) {
  const settings = plainObject(body.settings);
  const contents = Array.isArray(body.contents) ? body.contents : [];
  let videoInput = "none";
  for (const item of contents) {
    if (item && (item.type === "feature_video" || item.type === "base_video")) {
      videoInput = "video";
      break;
    }
    if (item && item.type === "element") videoInput = "unknown";
  }
  return { kling4: true, native: true, resolution: kling4Resolution(settings), videoInput: videoInput };
}

// The task's entry in a Kling 4.0 reply: the list GET /tasks answers, or the
// single task of the create reply.
function kling4Entry(reply, taskId) {
  const data = plainObject(reply).data;
  if (!Array.isArray(data)) return plainObject(data);
  return (
    data.find(function (entry) {
      return entry && taskId && entry.id === taskId;
    }) || plainObject(data[0])
  );
}

// A Kling 4.0 reply carries the task as data, or as the only entry of the list
// GET /tasks answers; the earlier API's reply has data.task_id.
function isKling4Reply(reply) {
  const data = plainObject(reply).data;
  return Array.isArray(data) || Boolean(data && typeof data === "object" && data.id && !data.task_id);
}

function kling4Videos(entry) {
  return (Array.isArray(entry.outputs) ? entry.outputs : []).filter(function (output) {
    return output && output.type === "video";
  });
}

// Upstream durations are saturated before they become units.
function reportedSeconds(value) {
  const seconds = Number.parseFloat(value);
  return Number.isFinite(seconds) && seconds > 0 ? Math.min(seconds, 3600) : 0;
}

// The units Kling deducted. A cash deduction counts its list_price, not its
// amount, which is the discounted price; the durations Kling reports run
// slightly longer than the requested duration it bills.
function kling4CompletedUnits(ctx, body) {
  const entry = kling4Entry(body, ctx && ctx.taskId);
  if (entry.status !== "succeeded") return null;
  const deductions = Array.isArray(entry.billing) ? entry.billing : [];
  const deducted = deductions.reduce(function (sum, item) {
    const type = item && item.charge_type;
    const units = Number.parseFloat(type === "unit" ? item.amount : type === "cash" ? item.list_price : "");
    return units >= 0 ? sum + units : Number.NaN;
  }, 0);
  if (deductions.length && Number.isFinite(deducted)) return { units: deducted };
  const videos = kling4Videos(entry);
  const output = reportedSeconds(videos.length ? videos[0].duration : "");
  if (!output) return null;
  const input = reportedSeconds(plainObject(entry.usage).input_duration);
  const resolution = kling4Resolution(plainObject(ctx && ctx.state));
  if (!input) return { units: KLING4_UNITS[resolution] * output };
  return { units: KLING4_VIDEO_INPUT_UNITS[resolution] * (input <= output ? output * 2 : input + output) };
}

function decodeKling4Submit(endpoint) {
  return function (ctx) {
    if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
    const body = ctx.body.value;
    if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("request body must be an object");
    checkKling4Body(body);
    return { kind: "submit", model: "kling-4.0", requestBody: { model: "kling-4.0", vendorBody: body, endpoint: endpoint } };
  };
}

// Kling's own body, whose model_name is required, sent as is for Kling to
// check.
function decodeNativeSubmit(ctx) {
  if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
  const body = ctx.body.value;
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("request body must be an object");
  const model = typeof body.model_name === "string" ? body.model_name.trim() : "";
  if (!model) throw new Error("model_name is required");
  for (const name of UNSUPPORTED_FIELDS) {
    if (body[name] !== undefined && body[name] !== null) throw new Error(name + " is not supported; query the task instead");
  }
  if (body.duration !== undefined && body.duration !== null) validateKlingDuration(body.duration);
  return { kind: "submit", model: model, requestBody: { model: model, vendorBody: body } };
}

export const native = {
  decodeSubmit: decodeNativeSubmit,
  taskCreated: function (ctx, task) {
    const result = task.data || {},
      data = result.data || {};
    return Object.assign({}, result, { data: Object.assign({}, data, { task_id: task.task_id }) });
  },
  // Kling's last reply for the task. The gateway can fail a task Kling still
  // reports as running (poll failures or the task timeout); it then reads failed.
  taskStatus: function (ctx, task) {
    const statusMap = { NOT_START: "submitted", SUBMITTED: "submitted", QUEUED: "submitted", IN_PROGRESS: "processing", SUCCESS: "succeed", FAILURE: "failed" };
    const result = task.data && typeof task.data === "object" && !Array.isArray(task.data) && !isKling4Reply(task.data) ? task.data : null;
    if (result) {
      const data = Object.assign({}, result.data && typeof result.data === "object" ? result.data : {}, { task_id: task.task_id });
      if (task.status === "FAILURE" && data.task_status !== "failed") {
        data.task_status = "failed";
        data.task_status_msg = task.fail_reason || "";
      }
      return Object.assign({}, result, { data: data });
    }
    return {
      code: 0,
      message: "",
      request_id: "",
      data: {
        task_id: task.task_id,
        task_status: statusMap[task.status] || "submitted",
        task_status_msg: task.fail_reason || "",
        created_at: task.created_at * 1000,
        updated_at: (task.updated_at || task.created_at) * 1000,
      },
    };
  },
  kling4TextToVideo: decodeKling4Submit("text-to-video"),
  kling4ImageToVideo: decodeKling4Submit("image-to-video"),
  kling4OmniVideo: decodeKling4Submit("omni-video"),
  kling4Created: function (ctx, task) {
    const result = plainObject(task.data);
    return Object.assign({}, result, { data: Object.assign({}, plainObject(result.data), { id: task.task_id }) });
  },
  // Kling 4.0 answers GET /tasks with a list; the gateway answers for one
  // task, from Kling's last reply. A task the gateway failed reads failed.
  kling4Tasks: function (ctx, task) {
    const statusMap = {
      NOT_START: "submitted",
      SUBMITTED: "submitted",
      QUEUED: "submitted",
      IN_PROGRESS: "processing",
      SUCCESS: "succeeded",
      FAILURE: "failed",
    };
    const result = isKling4Reply(task.data) ? plainObject(task.data) : {};
    const entry = Object.assign(
      { status: statusMap[task.status] || "submitted", create_time: task.created_at * 1000, update_time: (task.updated_at || task.created_at) * 1000 },
      kling4Entry(result),
      { id: task.task_id },
    );
    if (task.status === "FAILURE" && entry.status !== "failed") {
      entry.status = "failed";
      entry.message = task.fail_reason || "";
    }
    return Object.assign({ code: 0, message: "", request_id: "" }, result, { data: [entry] });
  },
  error: function (ctx, error) {
    return { code: ERROR_CODES[error.httpStatus] || (error.httpStatus >= 500 ? 5000 : 1200), message: error.message, request_id: error.requestId || "" };
  },
};

// Native routes send the client's own body with the channel's model. The host
// protocols add the prompt, first frame, mode and the aspect ratio nearest the
// size; image-to-video has no aspect ratio. They always send mode and
// duration, at Kling's defaults when absent.
export function buildSubmitRequest(ctx) {
  if (isKling4(ctx) && !ctx.requestBody.vendorBody) return buildKling4Submit(ctx);
  const req = ctx.requestBody;
  if (req.vendorBody && req.endpoint) {
    const model = submitModel(ctx, req);
    checkKling4Body(req.vendorBody);
    return {
      url: apiRoot(ctx) + "/" + req.endpoint + "/" + model,
      method: "POST",
      headers: Object.assign({ "Content-Type": "application/json" }, requestHeaders(ctx)),
      body: req.vendorBody,
      action: KLING4_ACTIONS[req.endpoint],
    };
  }
  const metadata = req.metadata || {};
  const inferredAction = req.image || metadata.image || metadata.image_tail ? "image_to_video" : "text_to_video";
  const action = ctx.action === "text_to_video" || ctx.action === "image_to_video" ? ctx.action : inferredAction;
  const model = submitModel(ctx, req);
  if (!model) throw new Error("model_name is required");
  let body;
  if (req.vendorBody) {
    body = Object.assign({}, req.vendorBody, { model_name: model });
    if (body.image === null) delete body.image;
  } else {
    body = {};
    if (req.prompt) body.prompt = req.prompt;
    if (req.image) body.image = req.image;
    if (req.mode) body.mode = req.mode;
    if (action === "text_to_video" && req.size !== undefined && req.size !== null && req.size !== "") body.aspect_ratio = aspectRatio(sizeDimensions(req.size), KLING_ASPECT_RATIOS);
    Object.assign(body, metadata);
    body.model_name = model;
    body.mode = resolveKlingMode(model, body.mode);
    body.duration = String(outboundDuration(req));
    if (body.image) body.image = filePlaceholder(body.image);
    if (body.image_tail) body.image_tail = filePlaceholder(body.image_tail);
    if (!body.prompt) delete body.prompt;
    if (!body.image) delete body.image;
  }
  return {
    url: urlFor(ctx, action),
    method: "POST",
    headers: Object.assign({ "Content-Type": "application/json" }, requestHeaders(ctx)),
    body: body,
    action: action,
  };
}

// A Kling 4.0 task keeps the resolution it was sent at, for a settlement
// without a unit deduction; the state also marks the task as one.
function isKling4Task(ctx) {
  return Boolean(plainObject(ctx && ctx.state).resolution);
}

export function parseSubmitResponse(ctx, resp) {
  if (isKling4(ctx) && !(ctx.requestBody || {}).vendorBody) {
    const result = resp.body || {};
    const gateway = ctx.upstream && ctx.upstream.kind === "new_api";
    if (!gateway && result.code !== 0) throw new Error(result.message || "kling submit failed");
    const id = gateway ? result.id || result.task_id : result.data && result.data.id;
    if (!id || typeof id !== "string") throw new Error("missing task id");
    return { taskId: id, taskData: result, state: Object.assign({}, ctx.state || {}, { upstreamTaskId: id }) };
  }
  const result = resp.body || {};
  if (result.code !== 0) throw new Error(result.message || "kling submit failed");
  const req = (ctx && ctx.requestBody) || {};
  if (req.vendorBody && req.endpoint) {
    if (!result.data || !result.data.id) throw new Error("missing task id");
    return { taskId: result.data.id, taskData: result, state: nativeKling4State(req.vendorBody) };
  }
  if (!result.data || !result.data.task_id) throw new Error("missing task_id");
  return { taskId: result.data.task_id, taskData: result };
}

export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return null;
  const req = ctx.requestBody || {};
  if (isKling4(ctx) && !req.vendorBody) {
    const state = ctx.state || kling4Payload(req).state;
    const output = state.duration === undefined ? 30 : state.duration;
    return { billable_seconds: state.videoInput === "none" ? output : 30 + output, resolution: state.resolution, video_input: state.videoInput === "none" ? "none" : "video" };
  }
  if (req.vendorBody && req.endpoint) {
    const state = nativeKling4State(req.vendorBody);
    const duration = plainObject(req.vendorBody.settings).duration;
    const output = duration == null ? KLING4_MAX_SECONDS : kling4Duration(duration);
    const hasVideo = state.videoInput !== "none";
    const seconds = hasVideo ? KLING4_MAX_SECONDS + output : output;
    const rate = (hasVideo ? KLING4_VIDEO_INPUT_UNITS : KLING4_UNITS)[state.resolution];
    return { units: seconds * rate, resolution: state.resolution, video_input: hasVideo ? "video" : "none", billable_seconds: seconds };
  }
  // A native body reads like the metadata of a host-protocol request.
  const source = req.vendorBody ? { metadata: req.vendorBody } : req;
  const model = submitModel(ctx, req);
  const rawMode = (source.metadata || {}).mode || source.mode;
  const mode = req.vendorBody && rawMode ? rawMode : resolveKlingMode(model, rawMode);
  return { units: unitsPerSecond(model, mode, source.metadata || {}) * outboundDuration(source) };
}

export function buildQueryRequest(ctx) {
  if ((ctx.state && ctx.state.native) || (!isKling4(ctx) && isKling4Task(ctx))) return { url: apiRoot(ctx) + "/tasks?task_ids=" + encodeURIComponent(ctx.taskId), method: "GET", headers: requestHeaders(ctx) };
  if (isKling4(ctx)) {
    const gateway = ctx.upstream && ctx.upstream.kind === "new_api";
    return {
      url: ctx.baseUrl + (gateway ? "/v1/videos/" + encodeURIComponent(ctx.taskId) : "/tasks?task_ids=" + encodeURIComponent(ctx.taskId)),
      method: "GET", headers: { Accept: "application/json", Authorization: "Bearer " + kling4Token(ctx) },
    };
  }
  return {
    url: urlFor(ctx, ctx.action) + "/" + ctx.taskId,
    method: "GET",
    headers: { Accept: "application/json", ...requestHeaders(ctx) },
  };
}

function parseKling4Result(ctx, body) {
  const entry = kling4Entry(body, ctx.taskId);
  const statuses = { submitted: "SUBMITTED", processing: "IN_PROGRESS", succeeded: "SUCCESS", failed: "FAILURE" };
  const status = statuses[entry.status];
  if (!status) return { status: "UNKNOWN", reason: "unknown task status: " + String(entry.status || "") };
  const result = { code: body.code || 0, taskId: entry.id, status: status, reason: entry.message || "" };
  const videos = status === "SUCCESS" ? kling4Videos(entry) : [];
  if (videos.length && videos[0].url) result.url = videos[0].url;
  const usage = kling4CompletedUnits(ctx, body);
  if (usage && usage.units > 0) {
    result.completionTokens = Math.ceil(usage.units);
    result.totalTokens = Math.ceil(usage.units);
  }
  return result;
}

export function parseTaskResult(ctx, body) {
  if (isKling4(ctx)) {
    const task = kling4Task(ctx, body);
    if (!task) return { status: "UNKNOWN", reason: "missing matching Kling task" };
    const statuses = { submitted: "SUBMITTED", queued: "QUEUED", processing: "IN_PROGRESS", in_progress: "IN_PROGRESS", succeeded: "SUCCESS", completed: "SUCCESS", failed: "FAILURE" };
    const status = statuses[task.status];
    if (!status) return { status: "UNKNOWN", reason: "unknown task status" };
    const result = { taskId: task.id, status: status, reason: task.message || (task.error && task.error.message) || "" };
    const outputs = task.outputs || [];
    for (const item of outputs) if (item.type === "video" && item.url) { result.url = item.url; break; }
    if (status === "SUCCESS") result.billingPending = !kling4Usage(ctx, body);
    return result;
  }
  if (isKling4Task(ctx)) return parseKling4Result(ctx, body);
  const data = body.data || {};
  const statuses = { submitted: "SUBMITTED", processing: "IN_PROGRESS", succeed: "SUCCESS", failed: "FAILURE" };
  const status = statuses[data.task_status];
  if (!status) return { status: "UNKNOWN", reason: "unknown task status: " + String(data.task_status || "") };
  const videos = status === "SUCCESS" && data.task_result && data.task_result.videos ? data.task_result.videos : [];
  const result = { code: body.code || 0, taskId: data.task_id, status: status, reason: data.task_status_msg || "" };
  if (videos.length && videos[0].url) result.url = videos[0].url;
  const units = Number.parseFloat(data.final_unit_deduction || "");
  if (Number.isFinite(units) && units > 0) {
    result.completionTokens = Math.ceil(units);
    result.totalTokens = Math.ceil(units);
  }
  return result;
}

function artifactData(ctx) {
  const data = (ctx && ctx.data) || {};
  if (data.data && typeof data.data === "object" && data.data.task_id && Object.prototype.hasOwnProperty.call(data.data, "data")) return data.data.data || {};
  return data;
}

function artifactVideoURL(ctx) {
  const snapshot = artifactData(ctx);
  const tasks = Array.isArray(snapshot.data) ? snapshot.data : snapshot.data && snapshot.data.result;
  if (Array.isArray(tasks)) {
    for (const task of tasks) {
      const upstreamId = ctx.upstreamTaskId || (ctx.state && ctx.state.upstreamTaskId) || ctx.taskId;
      if (upstreamId ? task.id !== upstreamId : tasks.length !== 1) continue;
      for (const output of task.outputs || []) if (output.type === "video" && output.url) return String(output.url);
    }
    return "";
  }
  if (snapshot.object === "video") return trimmed((snapshot.metadata || {}).url);

  if (isKling4Task(ctx)) {
    const videos = kling4Videos(kling4Entry(artifactData(ctx), ctx.taskId));
    return videos.length ? String(videos[0].url || "").trim() : "";
  }
  const result = (artifactData(ctx).data || {}).task_result || {};
  const videos = Array.isArray(result.videos) ? result.videos : [];
  return videos.length ? String(videos[0].url || "").trim() : "";
}

export function listArtifacts(task) {
  return task.status === "SUCCESS" && (artifactVideoURL(task) || (task.state && task.state.kling4 && task.state.gateway)) ? [{ key: "video", type: "video" }] : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  if (isKling4(ctx) && !(ctx.state && ctx.state.native) && ctx.upstream && ctx.upstream.kind === "new_api") {
    return { url: ctx.baseUrl + "/v1/videos/" + encodeURIComponent(ctx.upstreamTaskId) + "/content", method: ctx.clientRequest.method,
      headers: { Authorization: "Bearer " + kling4Token(ctx) } };
  }
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const url = artifactVideoURL(ctx);
  if (!url) throw new Error("artifact_not_found");
  return { url: url, method: ctx.clientRequest.method, credentialless: true };
}

export function extractUsageOnComplete(_task, _taskResult, body) {
  if (isKling4(_task)) return kling4Usage(_task, body);
  if (isKling4Task(_task)) return kling4CompletedUnits(_task, body);
  const data = (body && body.data) || {};
  if (
    !Object.prototype.hasOwnProperty.call(data, "final_unit_deduction") ||
    data.final_unit_deduction === undefined ||
    data.final_unit_deduction === null ||
    data.final_unit_deduction === ""
  ) {
    return null;
  }
  const units = Number.parseFloat(data.final_unit_deduction);
  if (!Number.isFinite(units)) return null;
  return { units: units };
}

export const protocols = {
  openai_responses: {
    decodeRequest: function (ctx) {
      if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
      const req = ctx.body.value;
      if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
      const model = trimmed(ctx.model);
      if (!model) throw new Error("model is required");
      if (req.input !== undefined && typeof req.input !== "string" && !Array.isArray(req.input)) throw new Error("input must be a string or array");
      if (req.images !== undefined && !Array.isArray(req.images)) throw new Error("images must be an array");
      if (req.metadata !== undefined && (!req.metadata || typeof req.metadata !== "object" || Array.isArray(req.metadata)))
        throw new Error("metadata must be an object");
      const input = responsesInput(req);
      const prompt = input.prompt || trimmed(req.prompt);
      const images = [];
      for (const image of [req.image, req.input_reference].concat(req.images || [], input.images)) {
        if (trimmed(image) && !images.includes(trimmed(image))) images.push(trimmed(image));
      }
      if (!prompt && images.length === 0) throw new Error("input is required");
      const metadata = Object.assign({}, req.metadata || {});
      if (Object.prototype.hasOwnProperty.call(req, "mode")) metadata.mode = req.mode;
      metadata.mode = klingMode(metadata.mode);
      if (images.length > 1 && !metadata.image_tail) metadata.image_tail = images[1];
      const requestBody = { model: model, prompt: prompt, metadata: metadata };
      if (images.length) requestBody.image = images[0];
      if (Object.prototype.hasOwnProperty.call(req, "seconds")) requestBody.duration = req.seconds;
      else if (Object.prototype.hasOwnProperty.call(req, "duration")) requestBody.duration = req.duration;
      if (Object.prototype.hasOwnProperty.call(req, "size")) requestBody.size = req.size;
      return { kind: "submit", model: model, action: images.length ? "image_to_video" : "text_to_video", requestBody: requestBody };
    },
    renderEvents: function (ctx, task, previousState) {
      const status = String(task.status || "UNKNOWN").toUpperCase();
      const value = Number(String(task.progress || "").replace("%", ""));
      const progress = Number.isFinite(value) && value >= 0 && value <= 100 ? value : null;
      const state = { status: status, progress: progress };
      if (status === "SUCCESS") {
        const text = responsesVideoText(ctx);
        const events = previousState && previousState.status === status ? [] : [{ type: "output", data: text }];
        return { events: events, state: state, done: true };
      }
      if (status === "FAILURE")
        return { events: [{ type: "error", code: "task_failed", message: task.fail_reason || "task failed" }], state: state, done: true };
      if (previousState && previousState.status === status && previousState.progress === progress) return { events: [], state: state, done: false };
      const event = { type: "progress", message: status.toLowerCase() };
      if (progress !== null) event.progress = progress;
      return { events: [event], state: state, done: false };
    },
    renderFinal: function (ctx, _task) {
      return {
        output: [
          {
            type: "message",
            status: "completed",
            role: "assistant",
            content: [{ type: "output_text", text: responsesVideoText(ctx), annotations: [], logprobs: [] }],
          },
        ],
        metadata: { vendor: "kling" },
      };
    },
  },
  openai_video: {
    decodeRequest: function (ctx) {
      if (isKling4(ctx)) return decodeKling4Video(ctx);
      if (!ctx.body || (ctx.body.kind !== "json" && ctx.body.kind !== "multipart")) throw new Error("JSON or multipart body required");
      let req;
      let hasInputReferenceFile = false;
      if (ctx.body.kind === "json") {
        if (!ctx.body.value || Array.isArray(ctx.body.value)) throw new Error("JSON object required");
        req = Object.assign({}, ctx.body.value);
      } else {
        const first = function (name) {
          const values = (ctx.body.fields || {})[name] || [];
          if (values.length > 1) throw new Error(name + " must be provided once");
          return values[0];
        };
        req = {};
        const fields = ctx.body.fields || {};
        for (const name of Object.keys(fields)) {
          req[name] = first(name);
        }
        for (const file of ctx.body.files || []) {
          if (file.field !== "input_reference") throw new Error("unexpected file field: " + file.field);
          if (hasInputReferenceFile) throw new Error("input_reference must be provided once");
          hasInputReferenceFile = true;
        }
        if (req.metadata !== undefined) {
          let parsed;
          try {
            parsed = JSON.parse(req.metadata);
          } catch (e) {
            throw new Error("metadata must be a JSON object string");
          }
          if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("metadata must be a JSON object string");
          req.metadata = parsed;
        }
        if (req.seconds !== undefined) req.seconds = Number(req.seconds);
        else if (req.duration !== undefined) req.seconds = Number(req.duration);
      }
      const seconds = req.seconds === undefined ? req.duration : req.seconds;
      // Without a duration the request builder sends Kling's default, 5, and
      // Kling 4.0 picks the length itself.
      if (seconds !== undefined) req.duration = validateKlingDuration(seconds);
      if (hasInputReferenceFile) {
        req.image = { __fileRef: "request_file:input_reference", encoding: "base64", maxBytes: 10485760 };
      } else {
        const image = trimmed(req.input_reference || req.image);
        if (image) req.image = image;
      }
      const metadata = req.metadata || {};
      req.mode = klingMode(req.mode || metadata.mode);
      const hasImage = hasKlingImage(req, hasInputReferenceFile);
      // vendorBody carries a native request's own body; clients of this protocol cannot set it.
      delete req.vendorBody;
      return {
        kind: "submit",
        model: ctx.model,
        action: hasImage ? "image_to_video" : "text_to_video",
        requestBody: Object.assign({}, req, { model: ctx.model }),
      };
    },
    render: function (ctx, task) {
      if (isKling4(ctx) || (task.data || {}).object === "video" || ((task.data || {}).data || {}).result) {
        const raw = task.data || {};
        const tasks = Array.isArray(raw.data) ? raw.data : (raw.data || {}).result;
        const item = Array.isArray(tasks) ? tasks.find(function (entry) { return entry.id === task.task_id; }) || (tasks.length === 1 ? tasks[0] : {}) : raw;
        const output = { seconds: item.seconds === undefined ? "" : item.seconds, usage: item.usage };
        for (const video of item.outputs || []) if (video.type === "video") { output.seconds = video.duration || ""; break; }
        if (item.error) output.error = item.error;
        if (task.status === "FAILURE") output.error = { code: "task_failed", message: task.fail_reason || item.message || "task failed" };
        return output;
      }
      const response = task.data || {};
      const data = response.data || {};
      const statusMap = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
      const output = {
        id: task.task_id,
        object: "video",
        model: "",
        status: statusMap[task.status] || "unknown",
        progress: Number(String(task.progress || "0").replace("%", "")),
        created_at: data.created_at || 0,
      };
      // Kling 4.0 replies carry the task as data, or as the only entry of the
      // list GET /tasks answers.
      if (isKling4Reply(response)) {
        const entry = kling4Entry(response);
        const videos = kling4Videos(entry);
        output.created_at = entry.create_time || 0;
        if (entry.update_time) output.completed_at = entry.update_time;
        if (videos.length && videos[0].duration) output.seconds = videos[0].duration;
        if (response.code !== 0 && response.message) output.error = { message: response.message, code: String(response.code) };
        if (entry.status === "failed") output.error = { message: entry.message || "", code: "" };
        return output;
      }
      if (data.updated_at) output.completed_at = data.updated_at;
      const videos = data.task_result && data.task_result.videos ? data.task_result.videos : [];
      if (videos.length) {
        if (videos[0].duration) output.seconds = videos[0].duration;
      }
      if (response.code !== 0 && response.message) output.error = { message: response.message, code: String(response.code) };
      if (data.task_status === "failed") output.error = { message: data.task_status_msg, code: "" };
      return output;
    },
  },
};
