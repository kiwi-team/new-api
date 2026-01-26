/*
Copyright (C) 2025 QuantumNous
*/

import React, { useEffect, useState, useRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Avatar,
  Typography,
  Banner,
  Select,
  Input,
  Collapse,
  Tag,
  Spin,
  Empty,
  Tabs,
  TabPane,
  Switch,
  TextArea,
  Toast,
  Tooltip,
  Space,
  Divider,
  TagInput,
} from '@douyinfe/semi-ui';
import {
  IconClose,
  IconCopy,
  IconRefresh,
  IconSave,
  IconPlay,
  IconStop,
} from '@douyinfe/semi-icons';
import { Play, Clock, AlertCircle, CheckCircle, Zap, Key, Server, Code, Eye, EyeOff, ChevronDown, ChevronRight, Image as ImageIcon, Brain, Wrench } from 'lucide-react';
import { API, showError, showSuccess } from '../../../helpers';

const { Text, Title, Paragraph } = Typography;

// 请求格式配置
const REQUEST_FORMATS = {
  openai: {
    name: 'OpenAI Chat',
    path: '/v1/chat/completions',
    contentType: 'application/json',
  },
  openai_responses: {
    name: 'OpenAI Responses',
    path: '/v1/responses',
    contentType: 'application/json',
  },
  claude: {
    name: 'Claude',
    path: '/v1/messages',
    contentType: 'application/json',
  },
  gemini: {
    name: 'Gemini',
    path: '/v1beta/models/{model}:generateContent',
    contentType: 'application/json',
  },
};

// 预置标签
const PRESET_TAGS = [
  { label: '功能验证', value: '功能验证' },
  { label: 'bug复现', value: 'bug复现' },
  { label: '性能测试', value: '性能测试' },
  { label: '回归测试', value: '回归测试' },
  { label: '多模态', value: '多模态' },
  { label: 'tools调用', value: 'tools调用' },
  { label: 'thinking', value: 'thinking' },
  { label: '流式', value: '流式' },
];

// 默认请求体模板
const DEFAULT_BODIES = {
  openai: {
    model: 'gpt-4',
    messages: [
      { role: 'user', content: 'Hello!' }
    ],
    stream: false,
  },
  openai_responses: {
    model: 'gpt-4',
    input: 'Hello!',
    stream: false,
  },
  claude: {
    model: 'claude-3-opus-20240229',
    max_tokens: 1024,
    messages: [
      { role: 'user', content: 'Hello!' }
    ],
    stream: false,
  },
  gemini: {
    contents: [
      { role: 'user', parts: [{ text: 'Hello!' }] }
    ],
  },
};

// Base64 智能显示组件
const Base64Display = ({ content, onPreview }) => {
  const [expanded, setExpanded] = useState(false);
  
  // 检测是否是 base64 图片
  const base64Regex = /^data:image\/(png|jpeg|jpg|gif|webp);base64,([A-Za-z0-9+/=]+)$/;
  const match = content.match(base64Regex);
  
  if (!match) {
    // 检查是否是纯 base64 字符串（长度超过100且只包含base64字符）
    if (content.length > 100 && /^[A-Za-z0-9+/=]+$/.test(content)) {
      const sizeKB = Math.round(content.length * 0.75 / 1024);
      return (
        <span className="inline-flex items-center bg-blue-50 text-blue-600 px-2 py-1 rounded text-xs">
          <ImageIcon size={12} className="mr-1" />
          [Base64: {sizeKB}KB]
          <button 
            className="ml-1 text-blue-500 hover:text-blue-700"
            onClick={() => setExpanded(!expanded)}
          >
            {expanded ? <EyeOff size={12} /> : <Eye size={12} />}
          </button>
          {expanded && (
            <span className="block mt-1 break-all text-xs text-gray-500">
              {content.substring(0, 100)}...
            </span>
          )}
        </span>
      );
    }
    return <span>{content}</span>;
  }
  
  const [, imageType, base64Data] = match;
  const sizeKB = Math.round(base64Data.length * 0.75 / 1024);
  
  return (
    <span className="inline-flex items-center bg-green-50 text-green-600 px-2 py-1 rounded text-xs cursor-pointer hover:bg-green-100"
      onClick={() => onPreview && onPreview(content)}
    >
      <ImageIcon size={12} className="mr-1" />
      [📷 Base64 Image: {sizeKB}KB, {imageType.toUpperCase()}]
      <Eye size={12} className="ml-1" />
    </span>
  );
};

// JSON 编辑器组件（带 Base64 折叠）
const JsonEditor = ({ value, onChange, placeholder, readOnly = false }) => {
  const [error, setError] = useState(null);
  const [showFormatted, setShowFormatted] = useState(true);
  
  const handleChange = (newValue) => {
    onChange(newValue);
    try {
      if (newValue.trim()) {
        JSON.parse(newValue);
        setError(null);
      } else {
        setError(null);
      }
    } catch (e) {
      setError(e.message);
    }
  };
  
  const handleFormat = () => {
    try {
      const parsed = JSON.parse(value);
      onChange(JSON.stringify(parsed, null, 2));
      setError(null);
    } catch (e) {
      setError(e.message);
    }
  };
  
  const handleMinify = () => {
    try {
      const parsed = JSON.parse(value);
      onChange(JSON.stringify(parsed));
      setError(null);
    } catch (e) {
      setError(e.message);
    }
  };
  
  return (
    <div className="relative">
      {!readOnly && (
        <div className="absolute top-2 right-2 z-10 flex gap-1">
          <Tooltip content="格式化">
            <Button size="small" icon={<Code size={14} />} onClick={handleFormat} />
          </Tooltip>
          <Tooltip content="压缩">
            <Button size="small" onClick={handleMinify}>Min</Button>
          </Tooltip>
          <Tooltip content="复制">
            <Button size="small" icon={<IconCopy />} onClick={() => {
              navigator.clipboard.writeText(value);
              showSuccess('已复制');
            }} />
          </Tooltip>
        </div>
      )}
      <TextArea
        value={value}
        onChange={handleChange}
        placeholder={placeholder}
        autosize={{ minRows: 10, maxRows: 30 }}
        style={{ fontFamily: 'monospace', fontSize: 12 }}
        className={error ? 'border-red-500' : ''}
        disabled={readOnly}
      />
      {error && (
        <div className="text-red-500 text-xs mt-1 flex items-center">
          <AlertCircle size={12} className="mr-1" />
          JSON 语法错误: {error}
        </div>
      )}
    </div>
  );
};

// 流式响应展示组件
const StreamingResponse = ({ content, thinking, rawData, isStreaming, stats, onStop }) => {
  const contentRef = useRef(null);
  const rawRef = useRef(null);
  const [activeTab, setActiveTab] = useState('content');
  
  useEffect(() => {
    if (isStreaming) {
      if (activeTab === 'content' && contentRef.current) {
        contentRef.current.scrollTop = contentRef.current.scrollHeight;
      }
      if (activeTab === 'raw' && rawRef.current) {
        rawRef.current.scrollTop = rawRef.current.scrollHeight;
      }
    }
  }, [content, rawData, isStreaming, activeTab]);
  
  return (
    <div className="border rounded-lg overflow-hidden">
      <div className="bg-gray-50 px-3 py-2 flex items-center justify-between border-b">
        <div className="flex items-center">
          <Text strong>响应</Text>
          {isStreaming && (
            <Tag color="blue" className="ml-2">
              <Spin size="small" /> 接收中...
            </Tag>
          )}
        </div>
        {isStreaming && (
          <Button size="small" type="danger" icon={<IconStop />} onClick={onStop}>
            停止
          </Button>
        )}
      </div>
      
      {thinking && (
        <Collapse defaultActiveKey={['thinking']}>
          <Collapse.Panel header={
            <div className="flex items-center">
              <Brain size={14} className="mr-2 text-purple-500" />
              <Text>Thinking / Reasoning</Text>
            </div>
          } itemKey="thinking">
            <div className="bg-purple-50 p-3 rounded max-h-40 overflow-auto">
              <pre className="whitespace-pre text-sm text-purple-800">{thinking}</pre>
            </div>
          </Collapse.Panel>
        </Collapse>
      )}
      
      <Tabs activeKey={activeTab} onChange={setActiveTab} className="px-3 pt-2">
        <TabPane tab="聚合内容" itemKey="content">
          <div ref={contentRef} className="max-h-96 overflow-auto pb-3">
            {content ? (
              <pre className="whitespace-pre text-sm overflow-x-auto">{content}</pre>
            ) : (
              <Empty description="等待响应..." />
            )}
          </div>
        </TabPane>
        <TabPane tab="原始数据" itemKey="raw">
          <div ref={rawRef} className="max-h-96 overflow-auto pb-3">
            {rawData ? (
              <pre className="whitespace-pre text-xs font-mono bg-gray-50 p-2 rounded overflow-x-auto">{rawData}</pre>
            ) : (
              <Empty description="等待响应..." />
            )}
          </div>
        </TabPane>
      </Tabs>
      
      {stats && (
        <div className="bg-gray-50 px-3 py-2 border-t flex items-center gap-4 text-xs text-gray-500">
          {stats.ttfb && <span>TTFB: {stats.ttfb}ms</span>}
          {stats.duration && <span>总耗时: {stats.duration}ms</span>}
          {stats.chunks && <span>Chunks: {stats.chunks}</span>}
          {stats.bytes && <span>已接收: {(stats.bytes / 1024).toFixed(1)}KB</span>}
        </div>
      )}
    </div>
  );
};

// 自定义 Header 编辑组件
const HeadersEditor = ({ headers, onChange }) => {
  const addHeader = () => {
    onChange([...headers, { key: '', value: '' }]);
  };
  
  const removeHeader = (index) => {
    const newHeaders = headers.filter((_, i) => i !== index);
    onChange(newHeaders);
  };
  
  const updateHeader = (index, field, value) => {
    const newHeaders = [...headers];
    newHeaders[index][field] = value;
    onChange(newHeaders);
  };
  
  return (
    <div className="space-y-2">
      {headers.map((header, index) => (
        <div key={index} className="flex gap-2 items-center">
          <Input
            value={header.key}
            onChange={(v) => updateHeader(index, 'key', v)}
            placeholder="Header Name"
            style={{ width: 150 }}
          />
          <Input
            value={header.value}
            onChange={(v) => updateHeader(index, 'value', v)}
            placeholder="Header Value"
            className="flex-1"
          />
          <Button
            icon={<IconClose />}
            type="danger"
            theme="borderless"
            onClick={() => removeHeader(index)}
          />
        </div>
      ))}
      <Button size="small" onClick={addHeader}>+ 添加 Header</Button>
    </div>
  );
};

const DebugExecutor = () => {
  const { t } = useTranslation();
  
  // 调试模式: channel 或 key
  const [debugMode, setDebugMode] = useState('channel');
  
  // 渠道和 Key 列表
  const [channels, setChannels] = useState([]);
  const [keys, setKeys] = useState([]);
  const [selectedTarget, setSelectedTarget] = useState(null);
  
  // 请求格式
  const [requestFormat, setRequestFormat] = useState('openai');
  
  // API 配置（可编辑）
  const [baseUrl, setBaseUrl] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [requestPath, setRequestPath] = useState('/v1/chat/completions');
  
  // 请求体
  const [requestBody, setRequestBody] = useState(JSON.stringify(DEFAULT_BODIES.openai, null, 2));
  
  // 自定义 Headers
  const [customHeaders, setCustomHeaders] = useState([]);
  
  // 流式开关
  const [isStream, setIsStream] = useState(false);
  
  // 标签和备注
  const [tags, setTags] = useState([]);
  const [remark, setRemark] = useState('');
  
  // 执行状态
  const [executing, setExecuting] = useState(false);
  const [isStreaming, setIsStreaming] = useState(false);
  const abortControllerRef = useRef(null);
  
  // 响应数据
  const [response, setResponse] = useState(null);
  const [streamContent, setStreamContent] = useState('');
  const [streamThinking, setStreamThinking] = useState('');
  const [streamRawData, setStreamRawData] = useState('');
  const [streamStats, setStreamStats] = useState(null);
  
  // 历史标签建议
  const [tagSuggestions, setTagSuggestions] = useState([]);

  // 加载渠道列表
  const loadChannels = async () => {
    try {
      const res = await API.get('/api/debug/channels');
      if (res.data.success) {
        setChannels(res.data.data || []);
      }
    } catch (error) {
      // 回退到普通渠道列表
      try {
        const res = await API.get('/api/channel/channel-name-list');
        if (res.data.success) {
          setChannels(res.data.data || []);
        }
      } catch (e) {
        console.error('Failed to load channels:', e);
      }
    }
  };

  // 加载 Key 列表
  const loadKeys = async () => {
    try {
      const res = await API.get('/api/debug/keys');
      if (res.data.success) {
        setKeys(res.data.data || []);
      }
    } catch (error) {
      // 回退到普通 token 列表
      try {
        const res = await API.get('/api/token/?p=0&size=100');
        if (res.data.success) {
          setKeys(res.data.data || []);
        }
      } catch (e) {
        console.error('Failed to load keys:', e);
      }
    }
  };

  // 加载标签建议
  const loadTagSuggestions = async () => {
    try {
      const res = await API.get('/api/debug/tags');
      if (res.data.success) {
        setTagSuggestions(res.data.data || []);
      }
    } catch (error) {
      console.error('Failed to load tag suggestions:', error);
    }
  };

  useEffect(() => {
    loadChannels();
    loadKeys();
    loadTagSuggestions();
    
    // 检查是否有从测试数据页面传来的数据
    const templateData = sessionStorage.getItem('debug_template_data');
    if (templateData) {
      try {
        const data = JSON.parse(templateData);
        if (data.vendor) setRequestFormat(data.vendor);
        if (data.request_body) setRequestBody(data.request_body);
        sessionStorage.removeItem('debug_template_data');
      } catch (e) {
        console.error('Failed to parse template data:', e);
      }
    }
    
    // 检查是否有从日志页面传来的重新执行数据
    const rerunData = sessionStorage.getItem('debug_rerun_data');
    if (rerunData) {
      try {
        const data = JSON.parse(rerunData);
        if (data.debug_mode) setDebugMode(data.debug_mode);
        if (data.target_id) setSelectedTarget(data.target_id);
        if (data.request_format) setRequestFormat(data.request_format);
        if (data.request_body) setRequestBody(data.request_body);
        sessionStorage.removeItem('debug_rerun_data');
      } catch (e) {
        console.error('Failed to parse rerun data:', e);
      }
    }
  }, []);

  // 当选择目标时，自动填充配置
  useEffect(() => {
    if (!selectedTarget) return;
    
    if (debugMode === 'channel') {
      const channel = channels.find(c => c.id === selectedTarget);
      if (channel) {
        setBaseUrl(channel.base_url || '');
        setApiKey(channel.key || '');
      }
    } else {
      const key = keys.find(k => k.id === selectedTarget);
      if (key) {
        // 使用系统地址
        setBaseUrl(window.location.origin);
        setApiKey(key.key || '');
      }
    }
  }, [selectedTarget, debugMode, channels, keys]);

  // 当请求格式改变时，更新路径和默认请求体
  useEffect(() => {
    const format = REQUEST_FORMATS[requestFormat];
    if (format) {
      setRequestPath(format.path);
      setRequestBody(JSON.stringify(DEFAULT_BODIES[requestFormat], null, 2));
    }
  }, [requestFormat]);

  // 同步流式开关到请求体
  useEffect(() => {
    try {
      const body = JSON.parse(requestBody);
      if (requestFormat !== 'gemini') {
        body.stream = isStream;
        setRequestBody(JSON.stringify(body, null, 2));
      }
    } catch (e) {
      // 忽略解析错误
    }
  }, [isStream]);

  // 生成 cURL 命令
  const generateCurl = useCallback(() => {
    try {
      const url = `${baseUrl}${requestPath}`;
      const headers = {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${apiKey}`,
        ...Object.fromEntries(customHeaders.filter(h => h.key).map(h => [h.key, h.value])),
      };
      
      let curl = `curl -X POST '${url}'`;
      Object.entries(headers).forEach(([key, value]) => {
        curl += ` \\\n  -H '${key}: ${value}'`;
      });
      curl += ` \\\n  -d '${requestBody.replace(/'/g, "'\\''")}'`;
      
      navigator.clipboard.writeText(curl);
      showSuccess('cURL 命令已复制到剪贴板');
    } catch (e) {
      showError('生成 cURL 失败');
    }
  }, [baseUrl, requestPath, apiKey, customHeaders, requestBody]);

  // 停止流式请求
  const handleStop = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setIsStreaming(false);
    setExecuting(false);
  };

  // 解析 SSE 数据
  const parseSSELine = (line, format) => {
    if (!line.startsWith('data: ')) return null;
    const data = line.slice(6);
    if (data === '[DONE]') return { done: true };
    
    try {
      const json = JSON.parse(data);
      
      if (format === 'openai') {
        const delta = json.choices?.[0]?.delta;
        return {
          content: delta?.content || '',
          reasoning: delta?.reasoning_content || '',
          toolCalls: delta?.tool_calls || [],
        };
      } else if (format === 'openai_responses') {
        // OpenAI Responses API 流式格式
        if (json.type === 'response.output_text.delta') {
          return { content: json.delta || '' };
        }
        if (json.type === 'response.output_text.done') {
          return { content: '' };
        }
        if (json.type === 'response.done') {
          return { done: true };
        }
        // 兼容其他可能的格式
        if (json.output_text) {
          return { content: json.output_text };
        }
      } else if (format === 'claude') {
        if (json.type === 'content_block_delta') {
          const delta = json.delta;
          if (delta?.type === 'thinking_delta') {
            return { thinking: delta.thinking || '' };
          }
          if (delta?.type === 'text_delta') {
            return { content: delta.text || '' };
          }
        }
        if (json.type === 'message_stop') {
          return { done: true };
        }
      } else if (format === 'gemini') {
        const text = json.candidates?.[0]?.content?.parts?.[0]?.text || '';
        return { content: text };
      }
    } catch (e) {
      console.warn('SSE parse error:', e);
    }
    return null;
  };

  // 执行请求
  const handleExecute = async () => {
    if (!baseUrl) {
      showError('请输入 Base URL');
      return;
    }
    
    let parsedBody;
    try {
      parsedBody = JSON.parse(requestBody);
    } catch (e) {
      showError('请求体 JSON 格式错误');
      return;
    }
    
    setExecuting(true);
    setResponse(null);
    setStreamContent('');
    setStreamThinking('');
    setStreamRawData('');
    setStreamStats(null);
    
    const url = `${baseUrl}${requestPath}`;
    const headers = {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${apiKey}`,
      ...Object.fromEntries(customHeaders.filter(h => h.key).map(h => [h.key, h.value])),
    };
    
    const startTime = Date.now();
    let ttfb = null;
    let chunks = 0;
    let totalBytes = 0;
    
    try {
      // 通过后端代理执行请求
      const payload = {
        mode: debugMode,
        target_id: selectedTarget,
        format: requestFormat,
        base_url: baseUrl,
        api_key: apiKey,
        path: requestPath,
        method: 'POST',
        headers: headers,
        body: parsedBody,
        tags: tags,
        remark: remark,
      };
      
      if (isStream) {
        // 流式请求
        abortControllerRef.current = new AbortController();
        setIsStreaming(true);
        
        // 获取用户ID用于认证
        let userId = -1;
        try {
          const userStr = localStorage.getItem('user');
          if (userStr) {
            const user = JSON.parse(userStr);
            userId = user.id || -1;
          }
        } catch (e) {
          console.error('Failed to get user ID:', e);
        }
        
        const res = await fetch('/api/debug/execute/stream', {
          method: 'POST',
          headers: { 
            'Content-Type': 'application/json',
            'New-API-User': String(userId),
          },
          body: JSON.stringify(payload),
          signal: abortControllerRef.current.signal,
          credentials: 'include', // 携带认证 cookie
        });
        
        ttfb = Date.now() - startTime;
        
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        let content = '';
        let thinking = '';
        let rawData = '';
        
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          
          const chunk = decoder.decode(value, { stream: true });
          totalBytes += chunk.length;
          buffer += chunk;
          
          // 按行解析
          const lines = buffer.split('\n');
          buffer = lines.pop() || '';
          
          for (const line of lines) {
            if (!line.trim()) continue;
            chunks++;
            
            // 收集原始数据
            rawData += line + '\n';
            
            const parsed = parseSSELine(line, requestFormat);
            if (parsed) {
              if (parsed.done) break;
              if (parsed.content) content += parsed.content;
              if (parsed.thinking) thinking += parsed.thinking;
              if (parsed.reasoning) thinking += parsed.reasoning;
            }
          }
          
          setStreamContent(content);
          setStreamThinking(thinking);
          setStreamRawData(rawData);
          setStreamStats({
            ttfb,
            duration: Date.now() - startTime,
            chunks,
            bytes: totalBytes,
          });
        }
        
        setIsStreaming(false);
        setResponse({
          status: res.status,
          content: content,
          thinking: thinking,
          rawData: rawData,
          duration: Date.now() - startTime,
          ttfb,
          chunks,
        });
      } else {
        // 非流式请求
        const res = await API.post('/api/debug/execute', payload);
        
        if (res.data.success) {
          const data = res.data.data;
          setResponse({
            status: data.response_status,
            headers: data.response_headers,
            body: data.response_body,
            duration: data.duration_ms,
            error: data.error_message,
          });
          showSuccess('执行完成');
        } else {
          showError(res.data.message);
        }
      }
    } catch (error) {
      if (error.name === 'AbortError') {
        showSuccess('请求已中断');
      } else {
        showError('执行失败: ' + error.message);
        setResponse({
          error: error.message,
          duration: Date.now() - startTime,
        });
      }
    } finally {
      setExecuting(false);
      setIsStreaming(false);
    }
  };

  // 保存为测试数据
  const handleSaveAsTestData = async () => {
    try {
      const payload = {
        name: `调试数据 ${new Date().toLocaleString()}`,
        vendor: requestFormat,
        category: 'basic',
        request_body: requestBody,
        tags: tags.join(','),
      };
      
      const res = await API.post('/api/debug/test-data', payload);
      if (res.data.success) {
        showSuccess('已保存为测试数据');
      } else {
        showError(res.data.message);
      }
    } catch (error) {
      showError('保存失败');
    }
  };

  // 渲染响应内容
  const renderResponse = () => {
    if (isStreaming) {
      return (
        <StreamingResponse
          content={streamContent}
          thinking={streamThinking}
          rawData={streamRawData}
          isStreaming={isStreaming}
          stats={streamStats}
          onStop={handleStop}
        />
      );
    }
    
    if (!response) {
      return (
        <Empty
          image={<Play size={48} className="text-gray-300" />}
          description="执行结果将显示在这里"
        />
      );
    }
    
    return (
      <div className="space-y-4">
        {/* 状态摘要 */}
        <div className="flex items-center gap-4">
          <Tag color={response.status >= 200 && response.status < 300 ? 'green' : 'red'} size="large">
            {response.status || 'Error'}
          </Tag>
          <div className="flex items-center text-gray-500">
            <Clock size={14} className="mr-1" />
            {response.duration}ms
          </div>
          {response.ttfb && (
            <div className="flex items-center text-gray-500">
              <Zap size={14} className="mr-1" />
              TTFB: {response.ttfb}ms
            </div>
          )}
          {response.chunks && (
            <div className="flex items-center text-gray-500">
              Chunks: {response.chunks}
            </div>
          )}
        </div>
        
        {/* Thinking 内容 */}
        {response.thinking && (
          <Collapse defaultActiveKey={['thinking']}>
            <Collapse.Panel header={
              <div className="flex items-center">
                <Brain size={14} className="mr-2 text-purple-500" />
                <Text>Thinking / Reasoning</Text>
              </div>
            } itemKey="thinking">
              <div className="bg-purple-50 p-3 rounded overflow-auto">
                <pre className="whitespace-pre text-sm text-purple-800">{response.thinking}</pre>
              </div>
            </Collapse.Panel>
          </Collapse>
        )}
        
        {/* 响应体 - 使用 Tabs 展示聚合内容和原始数据 */}
        <Tabs defaultActiveKey="content">
          <TabPane tab="聚合内容" itemKey="content">
            <div className="overflow-auto max-h-96">
              <pre className="bg-gray-50 p-3 rounded text-sm whitespace-pre">
                {response.content || (() => {
                  try {
                    return JSON.stringify(JSON.parse(response.body), null, 2);
                  } catch {
                    return response.body || '';
                  }
                })()}
              </pre>
            </div>
          </TabPane>
          {response.rawData && (
            <TabPane tab="原始数据" itemKey="raw">
              <div className="overflow-auto max-h-96">
                <pre className="bg-gray-50 p-3 rounded text-xs font-mono whitespace-pre">
                  {response.rawData}
                </pre>
              </div>
            </TabPane>
          )}
        </Tabs>
        
        {/* 错误信息 */}
        {response.error && (
          <Banner type="danger" description={response.error} />
        )}
      </div>
    );
  };

  return (
    <div className="mt-[60px] px-4 pb-6">
      <Card className="!rounded-2xl shadow-sm border-0 mb-6">
        {/* Header */}
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center">
            <Avatar size="large" color="green" className="mr-3 shadow-md">
              <Play size={24} />
            </Avatar>
            <div>
              <Title heading={3} className="m-0">{t('调试器')}</Title>
              <Text type="tertiary" size="small">
                {t('调试渠道或 Key，支持流式请求和多种格式')}
              </Text>
            </div>
          </div>
          <Space>
            <Button icon={<IconCopy />} onClick={generateCurl}>复制 cURL</Button>
            <Button icon={<IconSave />} onClick={handleSaveAsTestData}>保存为测试数据</Button>
          </Space>
        </div>

        {/* Banner */}
        <Banner
          type="info"
          description={t('选择调试模式和目标，编辑请求体后执行。支持 OpenAI/Claude/Gemini 格式，支持流式请求。')}
          className="!rounded-lg mb-4"
        />

        <div className="flex gap-6">
          {/* 左侧配置面板 */}
          <div className="w-1/3 min-w-[350px] space-y-4">
            {/* 调试模式选择 */}
            <Card className="!rounded-lg">
              <Title heading={6} className="mb-3">调试模式</Title>
              <div className="flex gap-2 mb-4">
                <Button
                  theme={debugMode === 'channel' ? 'solid' : 'light'}
                  onClick={() => { setDebugMode('channel'); setSelectedTarget(null); }}
                  icon={<Server size={14} />}
                >
                  调试渠道
                </Button>
                <Button
                  theme={debugMode === 'key' ? 'solid' : 'light'}
                  onClick={() => { setDebugMode('key'); setSelectedTarget(null); }}
                  icon={<Key size={14} />}
                >
                  调试 Key
                </Button>
              </div>
              
              <div className="mb-4">
                <Text strong className="block mb-2">
                  {debugMode === 'channel' ? '选择渠道' : '选择 Key'}
                </Text>
                <Select
                  value={selectedTarget}
                  onChange={setSelectedTarget}
                  style={{ width: '100%' }}
                  placeholder={debugMode === 'channel' ? '请选择渠道' : '请选择 Key'}
                  filter
                  showClear
                >
                  {debugMode === 'channel' ? (
                    channels.map(channel => (
                      <Select.Option key={channel.id} value={channel.id}>
                        {channel.name} (ID: {channel.id})
                      </Select.Option>
                    ))
                  ) : (
                    keys.map(key => (
                      <Select.Option key={key.id} value={key.id}>
                        {key.name} (ID: {key.id})
                      </Select.Option>
                    ))
                  )}
                </Select>
              </div>
            </Card>

            {/* 请求格式 */}
            <Card className="!rounded-lg">
              <Title heading={6} className="mb-3">请求格式</Title>
              <Select
                value={requestFormat}
                onChange={setRequestFormat}
                style={{ width: '100%' }}
              >
                {Object.entries(REQUEST_FORMATS).map(([key, format]) => (
                  <Select.Option key={key} value={key}>
                    {format.name} - {format.path}
                  </Select.Option>
                ))}
              </Select>
            </Card>

            {/* API 配置 */}
            <Card className="!rounded-lg">
              <Title heading={6} className="mb-3">API 配置</Title>
              <div className="space-y-3">
                <div>
                  <Text strong className="block mb-1">Base URL</Text>
                  <Input
                    value={baseUrl}
                    onChange={setBaseUrl}
                    placeholder="https://api.openai.com"
                  />
                </div>
                <div>
                  <Text strong className="block mb-1">API Key</Text>
                  <Input
                    value={apiKey}
                    onChange={setApiKey}
                    placeholder="sk-..."
                    mode="password"
                  />
                </div>
                <div>
                  <Text strong className="block mb-1">请求路径</Text>
                  <Input
                    value={requestPath}
                    onChange={setRequestPath}
                    placeholder="/v1/chat/completions"
                  />
                </div>
              </div>
            </Card>

            {/* 流式开关 */}
            <Card className="!rounded-lg">
              <div className="flex items-center justify-between">
                <div className="flex items-center">
                  <Zap size={16} className="mr-2 text-yellow-500" />
                  <Text strong>流式请求</Text>
                </div>
                <Switch checked={isStream} onChange={setIsStream} />
              </div>
            </Card>

            {/* 标签和备注 */}
            <Card className="!rounded-lg">
              <Title heading={6} className="mb-3">标签和备注</Title>
              <div className="space-y-3">
                <div>
                  <Text strong className="block mb-1">标签</Text>
                  <TagInput
                    value={tags}
                    onChange={setTags}
                    placeholder="输入标签后按回车"
                    addOnBlur
                  />
                  <div className="mt-2 flex flex-wrap gap-1">
                    {PRESET_TAGS.map(tag => (
                      <Tag
                        key={tag.value}
                        size="small"
                        color="cyan"
                        className="cursor-pointer"
                        onClick={() => {
                          if (!tags.includes(tag.value)) {
                            setTags([...tags, tag.value]);
                          }
                        }}
                      >
                        + {tag.label}
                      </Tag>
                    ))}
                  </div>
                </div>
                <div>
                  <Text strong className="block mb-1">备注</Text>
                  <Input
                    value={remark}
                    onChange={setRemark}
                    placeholder="添加备注说明"
                  />
                </div>
              </div>
            </Card>

            {/* 自定义 Headers */}
            <Collapse>
              <Collapse.Panel header="自定义 Headers" itemKey="headers">
                <HeadersEditor headers={customHeaders} onChange={setCustomHeaders} />
              </Collapse.Panel>
            </Collapse>

            {/* 执行按钮 */}
            <Button
              theme="solid"
              type="primary"
              icon={executing ? <Spin size="small" /> : <Play size={16} />}
              loading={executing && !isStreaming}
              block
              size="large"
              onClick={handleExecute}
              disabled={executing}
            >
              {executing ? (isStreaming ? '接收中...' : '执行中...') : '执行请求'}
            </Button>
          </div>

          {/* 右侧内容区 */}
          <div className="flex-1 space-y-4">
            {/* 请求体编辑器 */}
            <Card className="!rounded-lg">
              <div className="flex items-center justify-between mb-3">
                <Title heading={6} className="m-0">请求体</Title>
                <Text type="tertiary" size="small">JSON 格式</Text>
              </div>
              <JsonEditor
                value={requestBody}
                onChange={setRequestBody}
                placeholder="输入 JSON 请求体..."
              />
            </Card>

            {/* 响应区域 */}
            <Card className="!rounded-lg">
              <div className="flex items-center justify-between mb-3">
                <Title heading={6} className="m-0">响应</Title>
                {response && !isStreaming && (
                  <Button
                    size="small"
                    icon={<IconClose />}
                    onClick={() => setResponse(null)}
                  >
                    清空
                  </Button>
                )}
              </div>
              {renderResponse()}
            </Card>
          </div>
        </div>
      </Card>
    </div>
  );
};

export default DebugExecutor;
