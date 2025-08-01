import React, { useEffect, useState, useRef } from 'react';
import { Button, Col, Form, Row, Spin, Banner } from '@douyinfe/semi-ui';
import {
    compareObjects,
    API,
    showError,
    showSuccess,
    showWarning,
    verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingChannelConfig(props) {
    const { t } = useTranslation();

    const [loading, setLoading] = useState(false);
    const [inputs, setInputs] = useState({
        'GlobalFirstChannelsSwitch': false,
        'GlobalFirstChannels': '',
    });
    const refForm = useRef();
    const [inputsRow, setInputsRow] = useState(inputs);

    function onSubmit() {
        const updateArray = compareObjects(inputs, inputsRow);
        if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
        const requestQueue = updateArray.map((item) => {
            let value = String(inputs[item.key]);

            return API.put('/api/option/', {
                key: item.key,
                value,
            });
        });
        setLoading(true);
        Promise.all(requestQueue)
            .then((res) => {
                if (requestQueue.length === 1) {
                    if (res.includes(undefined)) return;
                } else if (requestQueue.length > 1) {
                    if (res.includes(undefined))
                        return showError(t('部分保存失败，请重试'));
                }
                showSuccess(t('保存成功'));
                props.refresh();
            })
            .catch(() => {
                showError(t('保存失败，请重试'));
            })
            .finally(() => {
                setLoading(false);
            });
    }

    useEffect(() => {
        const currentInputs = {};
        for (let key in props.options) {
            if (Object.keys(inputs).includes(key)) {
                if (key === 'GlobalFirstChannelsSwitch') {
                    currentInputs[key] = props.options[key] === 'true';
                } else {
                    currentInputs[key] = props.options[key];
                }
            }
        }
        setInputs(currentInputs);
        setInputsRow(structuredClone(currentInputs));
        refForm.current.setValues(currentInputs);
    }, [props.options]);

    return (
        <>
            <Spin spinning={loading}>
                <Form
                    values={inputs}
                    getFormApi={(formAPI) => (refForm.current = formAPI)}
                    style={{ marginBottom: 15 }}
                >
                    <Form.Section text={t('渠道优先设置')}>
                        <Row>
                            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
                                <Form.Switch
                                    label={t('启用渠道优先设置')}
                                    field={'GlobalFirstChannelsSwitch'}
                                    onChange={(value) =>
                                        setInputs({
                                            ...inputs,
                                            'GlobalFirstChannelsSwitch': value,
                                        })
                                    }
                                    extraText={
                                        '开启后，会按照设置的优先渠道进行尝试，有多个渠道时，会按照设置的顺序进行尝试'
                                    }
                                />
                            </Col>
                        </Row>

                        <Form.Section text={t('设置渠道')}>
                            <Row style={{ marginTop: 10 }}>
                                <Col span={24}>
                                    <Banner
                                        type="warning"
                                        description="设置优先渠道，有多个渠道时，会按照设置的顺序进行尝试，该设置优先级低于渠道维度和key维度的配置"
                                    />
                                </Col>
                            </Row>
                            <Row>
                                <Form.Input
                                    label={t('设置优先渠道')}
                                    field={'GlobalFirstChannels'}
                                    onChange={(value) => setInputs({ ...inputs, 'GlobalFirstChannels': value })}
                                    min={1}
                                    disabled={!inputs['GlobalFirstChannelsSwitch']}
                                    placeholder="请输入渠道ID，多个渠道用逗号隔开"
                                />
                            </Row>
                        </Form.Section>

                        <Row>
                            <Button size='default' onClick={onSubmit}>
                                {t('保存')}
                            </Button>
                        </Row>
                    </Form.Section>
                </Form>
            </Spin>
        </>
    );
}
