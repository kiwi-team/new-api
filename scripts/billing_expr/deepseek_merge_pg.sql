-- DeepSeek 峰谷定价 (deepseek-v4-flash / deepseek-v4-pro)
-- 来源: https://api-docs.deepseek.com/zh-cn/quick_start/pricing/
-- 基价 = 空闲时段单价; 高峰 (北京时间 09:00-12:00 / 14:00-18:00) 乘 2
-- 两个窗口互斥, 任一时刻最多命中一条规则, 因此倍率非 1 即 2
-- 人民币按 1 USD = 6.8 CNY 折算为表达式系数 ($/1M tokens)
-- 仅合并这两个模型, 不影响 options 表中已有的其它模型配置

UPDATE options
SET "value" = (COALESCE(NULLIF("value", ''), '{}')::jsonb || '{"deepseek-v4-flash":"tiered_expr","deepseek-v4-pro":"tiered_expr"}'::jsonb)::text
WHERE "key" = 'billing_setting.billing_mode';

UPDATE options
SET "value" = (COALESCE(NULLIF("value", ''), '{}')::jsonb || '{"deepseek-v4-flash":"(tier(\"base\", p * 0.22058824 + c * 0.66176471 + cr * 0.00735294)) * (hour(\"Asia/Shanghai\") >= 9 && hour(\"Asia/Shanghai\") < 12 ? 2 : 1) * (hour(\"Asia/Shanghai\") >= 14 && hour(\"Asia/Shanghai\") < 18 ? 2 : 1)","deepseek-v4-pro":"(tier(\"base\", p * 0.66176471 + c * 1.98529412 + cr * 0.02205882)) * (hour(\"Asia/Shanghai\") >= 9 && hour(\"Asia/Shanghai\") < 12 ? 2 : 1) * (hour(\"Asia/Shanghai\") >= 14 && hour(\"Asia/Shanghai\") < 18 ? 2 : 1)"}'::jsonb)::text
WHERE "key" = 'billing_setting.billing_expr';
