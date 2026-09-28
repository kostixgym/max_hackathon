import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { Btn, Card, Cell, Foot, Header, IconButton, Main, Note, Opt, Screen, Tile, UiInput, UiList, useTheme } from '../../components/ui';
import type { IconName } from '../../components/Icon';
import { useApi } from '../../hooks/useApi';
import { createInitiative, fetchMe, fetchTemplate, fetchTemplates, type MeResponse } from '../../lib/api';
import { P } from '../../paths';

const templateIcons: Record<string, IconName> = {
  video_surveillance: 'video', intercom: 'intercom', barrier: 'barrier', custom: 'edit',
};
const paymentLabels: Record<string, string> = {
  management_bill: 'Строкой в квитанции УК',
  special_assessment: 'Разовым целевым сбором',
};
const accessLabels: Record<string, string> = {
  management_company: 'Управляющая компания',
  contractor: 'Обслуживающий подрядчик',
  house_council: 'Председатель совета дома',
};

type OwnerHouse = MeResponse['memberships'][number]['house'];

function ownerHouses(me: MeResponse | null): OwnerHouse[] {
  const houses = new Map<string, OwnerHouse>();
  for (const membership of me?.memberships ?? []) {
    if (membership.status === 'verified' && membership.role === 'owner') {
      houses.set(membership.house.slug, membership.house);
    }
  }
  return [...houses.values()];
}

function selectedHouse(houses: OwnerHouse[], requestedSlug: string | null): OwnerHouse | null {
  if (requestedSlug) return houses.find((house) => house.slug === requestedSlug) ?? null;
  return houses.length === 1 ? houses[0] : null;
}

function templatePath(code: string, houseSlug: string): string {
  return `/templates/${encodeURIComponent(code)}?house=${encodeURIComponent(houseSlug)}`;
}

function TemplateThemeSwitch() {
  const { theme, changeTheme } = useTheme();
  return (
    <div className="template-theme-row">
      <span className="template-theme-label">Тема</span>
      <div className="template-theme-switch" role="group" aria-label="Тема оформления">
        <button type="button" className={theme === 'light' ? 'selected' : ''} aria-pressed={theme === 'light'}
          onClick={() => changeTheme('light')}>Светлая</button>
        <button type="button" className={theme === 'dark' ? 'selected' : ''} aria-pressed={theme === 'dark'}
          onClick={() => changeTheme('dark')}>Тёмная</button>
      </div>
    </div>
  );
}

export function Templates() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const requestedSlug = searchParams.get('house');
  const templatesState = useApi(() => fetchTemplates());
  const meState = useApi(() => fetchMe());

  if (templatesState.loading || meState.loading) {
    return <Screen><Header title="Новая инициатива" /><Main><Card className="card">Загружаем шаблоны…</Card></Main></Screen>;
  }
  if (templatesState.error || meState.error || !templatesState.data || !meState.data) {
    return <Screen><Header title="Новая инициатива" /><Main><Note kind="neg">Не удалось загрузить шаблоны. Попробуйте обновить страницу.</Note></Main></Screen>;
  }

  const houses = ownerHouses(meState.data);
  const house = selectedHouse(houses, requestedSlug);
  const templates = templatesState.data.templates;

  return (
    <Screen>
      <Header title="Новая инициатива" onNav={() => navigate('/')} />
      <Main className="template-main">
        <div className="template-intro">
          <MaxTypography.Headline className="template-title" variant="medium">Начните с готового решения</MaxTypography.Headline>
          <MaxTypography.Text variant="body" color="secondary">Шаблон подскажет, что обсудить с соседями и какие вопросы вынести на собрание.</MaxTypography.Text>
        </div>
        <TemplateThemeSwitch />

        {houses.length === 0 ? (
          <><Note kind="info">Сначала подтвердите квартиру, чтобы создать инициативу.</Note><Btn kind="secondary" to="/attach">Добавить квартиру</Btn></>
        ) : (
          <Card className="card template-house-card">
            <MaxTypography.Label className="template-eyebrow" variant="medium-strong">ДОМ ИНИЦИАТИВЫ</MaxTypography.Label>
            {houses.length === 1 ? (
              <MaxTypography.Title variant="medium-strong">{houses[0].address}</MaxTypography.Title>
            ) : (
              <div className="template-house-list" role="group" aria-label="Выберите дом для инициативы">
                {houses.map((item) => (
                  <button key={item.id} type="button" className={'template-house-option' + (house?.id === item.id ? ' selected' : '')}
                    aria-pressed={house?.id === item.id}
                    onClick={() => navigate(`${P.templates}?house=${encodeURIComponent(item.slug)}`, { replace: true })}>
                    <span className="template-house-radio" aria-hidden="true" />
                    <span>{item.address}</span>
                  </button>
                ))}
              </div>
            )}
            {requestedSlug && !house && (
              <>
                <Note kind="neg">У вас нет подтверждённой квартиры в выбранном доме.</Note>
                {houses.length === 1 && <Btn kind="secondary" to={P.templates}>Использовать мой дом</Btn>}
              </>
            )}
          </Card>
        )}

        <div className="template-section-heading">
          <MaxTypography.Headline variant="small">Шаблоны</MaxTypography.Headline>
          <MaxTypography.Text variant="detail" color="secondary">{templates.length}</MaxTypography.Text>
        </div>
        {houses.length > 1 && !house && <Note kind="info">Выберите дом выше, чтобы открыть шаблон.</Note>}
        {templates.length > 0 ? (
          <UiList className="template-list">
            {templates.map((template) => {
              const supported = template.code === 'video_surveillance';
              return (
                <Cell key={template.code}
                  lead={<Tile icon={templateIcons[template.code] || 'edit'} style={{ width: 52, height: 52 }} />}
                  chevron={Boolean(house && supported)}
                  onClick={house && supported ? () => navigate(templatePath(template.code, house.slug)) : undefined}>
                  <span className="template-list-copy">
                    <MaxTypography.Title variant="medium-strong">{template.name}</MaxTypography.Title>
                    <MaxTypography.Text variant="detail" color="secondary">{template.description}</MaxTypography.Text>
                    {!supported && <MaxTypography.Text variant="detail" color="secondary">Форма пока не готова</MaxTypography.Text>}
                  </span>
                </Cell>
              );
            })}
          </UiList>
        ) : <Note kind="info">Шаблоны пока не добавлены.</Note>}
        <Note kind="info">Сначала создастся черновик. Его будете видеть только вы.</Note>
      </Main>
    </Screen>
  );
}

export function TemplateForm() {
  const { code } = useParams<{ code: string }>();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const requestedSlug = searchParams.get('house');
  const templateState = useApi(() => (code ? fetchTemplate(code) : Promise.reject(new Error('Шаблон не выбран'))), [code]);
  const meState = useApi(() => fetchMe());
  const [title, setTitle] = useState('Видеонаблюдение в подъездах');
  const [placement, setPlacement] = useState('Входы в подъезды и лифтовые холлы');
  const [count, setCount] = useState(6);
  const [cost, setCost] = useState('');
  const [payment, setPayment] = useState('management_bill');
  const [records, setRecords] = useState('management_company');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState('');

  if (templateState.loading || meState.loading) {
    return <Screen><Header title="Новая инициатива" /><Main><Card className="card">Загружаем форму…</Card></Main></Screen>;
  }
  if (templateState.error || !templateState.data || meState.error || !meState.data) {
    return <Screen><Header title="Новая инициатива" /><Main><Note kind="neg">Не удалось загрузить шаблон или профиль.</Note></Main></Screen>;
  }

  const template = templateState.data;
  const house = selectedHouse(ownerHouses(meState.data), requestedSlug);
  const backPath = `${P.templates}${requestedSlug ? `?house=${encodeURIComponent(requestedSlug)}` : ''}`;
  if (!house) {
    return <Screen><Header title={template.name} onNav={() => navigate(backPath)} /><Main><Note kind="info">Выберите дом с подтверждённой квартирой, чтобы создать инициативу.</Note><Btn to={P.templates}>Выбрать дом</Btn></Main></Screen>;
  }
  if (template.code !== 'video_surveillance') {
    return <Screen><Header title={template.name} onNav={() => navigate(backPath)} /><Main><Note kind="info">Форма для этого шаблона пока не готова.</Note><Btn to={backPath}>К шаблонам</Btn></Main></Screen>;
  }

  const trimmedTitle = title.trim();
  const trimmedPlacement = placement.trim();
  const costNumber = cost === '' ? null : Number(cost);
  const costInvalid = costNumber !== null && (!Number.isSafeInteger(costNumber) || costNumber > 100_000_000);
  const canSubmit = Boolean(trimmedTitle && trimmedPlacement && count >= 1 && count <= 1000 && !costInvalid);

  const submit = async () => {
    if (!canSubmit || submitting) return;
    setSubmitting(true);
    setSubmitError('');
    try {
      const params = {
        placement: trimmedPlacement,
        camera_count: count,
        ...(costNumber !== null ? { estimated_cost_rub: costNumber } : {}),
        payment_method: payment,
        records_access: records,
      };
      const description = `${trimmedPlacement}. ${count} камер. Оплата: ${paymentLabels[payment].toLowerCase()}. Доступ к записям: ${accessLabels[records].toLowerCase()}.`;
      const created = await createInitiative(house.id, { template_code: template.code, title: trimmedTitle, description, params });
      navigate(`/initiatives/${created.id}`, { replace: true });
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : 'Не удалось создать инициативу. Попробуйте ещё раз.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Screen>
      <Header title="Новая инициатива" onNav={() => navigate(backPath)} right={<span className="template-draft-badge">Черновик</span>} />
      <Main className="template-main">
        <div className="template-intro">
          <MaxTypography.Label className="template-eyebrow" variant="medium-strong">ШАБЛОН · ВИДЕОНАБЛЮДЕНИЕ</MaxTypography.Label>
          <MaxTypography.Headline className="template-title" variant="medium">Как будут работать камеры</MaxTypography.Headline>
          <MaxTypography.Text variant="body" color="secondary">Уточните проект для {house.address}. Эти детали увидят соседи перед опросом.</MaxTypography.Text>
        </div>
        <TemplateThemeSwitch />

        <Card className="card template-section">
          <div className="template-section-heading"><span className="template-section-number">1</span><MaxTypography.Headline variant="small">Об инициативе</MaxTypography.Headline></div>
          <label className="field">
            <MaxTypography.Label className="lbl" variant="large-strong">Название инициативы</MaxTypography.Label>
            <UiInput value={title} onChange={(event) => setTitle(event.target.value)} maxLength={160} placeholder="Например, камеры в подъездах" />
            <MaxTypography.Text variant="detail" color="secondary">Короткое название для списка и опроса · {title.length}/160</MaxTypography.Text>
          </label>
          <label className="field">
            <MaxTypography.Label className="lbl" variant="large-strong">Где установим камеры</MaxTypography.Label>
            <textarea className="template-textarea" value={placement} onChange={(event) => setPlacement(event.target.value)} maxLength={300}
              placeholder="Например, у входов и в лифтовых холлах" rows={3} />
            <MaxTypography.Text variant="detail" color="secondary">Укажите места установки · {placement.length}/300</MaxTypography.Text>
          </label>
        </Card>

        <Card className="card template-section">
          <div className="template-section-heading"><span className="template-section-number">2</span><MaxTypography.Headline variant="small">Параметры проекта</MaxTypography.Headline></div>
          <div className="field">
            <MaxTypography.Label className="lbl" variant="large-strong">Количество камер</MaxTypography.Label>
            <div className="template-stepper">
              <IconButton icon="minus" label="Уменьшить количество камер" onClick={() => setCount((value) => Math.max(1, value - 1))} />
              <b className="template-stepper-value" aria-live="polite">{count}</b>
              <IconButton icon="plus" label="Увеличить количество камер" onClick={() => setCount((value) => Math.min(1000, value + 1))} />
            </div>
            <MaxTypography.Text variant="detail" color="secondary">От 1 до 1000 камер</MaxTypography.Text>
          </div>
          <label className="field">
            <MaxTypography.Label className="lbl" variant="large-strong">Ориентир общей стоимости, ₽</MaxTypography.Label>
            <UiInput inputMode="numeric" value={cost} onChange={(event) => setCost(event.target.value.replace(/\D/g, ''))}
              placeholder="Необязательно" aria-invalid={costInvalid} />
            <MaxTypography.Text className={costInvalid ? 'template-error-text' : ''} variant="detail" color={costInvalid ? undefined : 'secondary'}>
              {costInvalid ? 'Укажите сумму не больше 100 000 000 ₽' : 'Оценку можно добавить позже'}
            </MaxTypography.Text>
          </label>
        </Card>

        <Card className="card template-section">
          <div className="template-section-heading"><span className="template-section-number">3</span><MaxTypography.Headline variant="small">Оплата и записи</MaxTypography.Headline></div>
          <div className="template-choice-group" role="group" aria-label="Способ оплаты">
            <MaxTypography.Label className="lbl" variant="large-strong">Как оплачиваем</MaxTypography.Label>
            {Object.entries(paymentLabels).map(([value, label]) => <Opt key={value} on={payment === value} onClick={() => setPayment(value)} title={label} />)}
          </div>
          <div className="template-choice-group" role="group" aria-label="Доступ к записям">
            <MaxTypography.Label className="lbl" variant="large-strong">Кто получит доступ к записям</MaxTypography.Label>
            {Object.entries(accessLabels).map(([value, label]) => <Opt key={value} on={records === value} onClick={() => setRecords(value)} title={label} />)}
          </div>
        </Card>

        <Card className="card template-section">
          <div className="template-section-heading"><span className="template-section-number">4</span><MaxTypography.Headline variant="small">Повестка собрания</MaxTypography.Headline></div>
          <MaxTypography.Text variant="body" color="secondary">Эти вопросы появятся в бюллетене. Формулировки и правила подсчёта уже подготовлены.</MaxTypography.Text>
          <div className="template-agenda">
            {template.agenda_items.map((item) => (
              <div key={item.position} className="template-agenda-item">
                <span className="template-agenda-number">{item.position}</span>
                <div className="template-agenda-copy">
                  <MaxTypography.Text variant="body">{item.text}</MaxTypography.Text>
                  <MaxTypography.Text variant="detail" color="secondary">Правило: {item.majority_rule}</MaxTypography.Text>
                </div>
              </div>
            ))}
          </div>
        </Card>
        {submitError && <Note kind="neg">{submitError}</Note>}
      </Main>
      <Foot>
        <Btn onClick={submit} disabled={submitting || !canSubmit}>{submitting ? 'Создаём черновик…' : 'Создать черновик'}</Btn>
        <MaxTypography.Text className="template-footer-hint" variant="detail" color="secondary">Черновик видите только вы. Опрос начнётся после вашего решения.</MaxTypography.Text>
      </Foot>
    </Screen>
  );
}
