import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Badge, Btn, Card, Foot, Header, Kv, Main, Note, Opt, Screen, Status, UiInput } from '../../components/ui';
import { useApi } from '../../hooks/useApi';
import { createDemand, downloadDemandPDF, fetchDemand, fetchInitiative, fetchMe, fetchPollProgress, markDemandDelivered, parseM2, selectPathB, type DemandRecord } from '../../lib/api';
import { fmtM2 } from '../../lib/format';

function dateText(value: string | null): string {
  return value ? new Date(value).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' }) : '—';
}

function todayInputDate(): string {
  const date = new Date();
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

function ErrorNote({ error }: { error: unknown }) {
  return error ? <Note kind="neg">{error instanceof Error ? error.message : 'Не удалось выполнить действие'}</Note> : null;
}

export function PathChoicePage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const initiativeState = useApi(() => id ? fetchInitiative(id) : Promise.reject(new Error('Инициатива не выбрана')), [id]);
  const pollState = useApi(() => id ? fetchPollProgress(id) : Promise.reject(new Error('Опрос не выбран')), [id]);
  if (initiativeState.loading || pollState.loading) return <Screen><Header title="Кто проведёт собрание" /><Main><Card className="card">Загружаем результаты опроса…</Card></Main></Screen>;
  if (initiativeState.error || pollState.error || !initiativeState.data || !pollState.data || !id) return <Screen><Header title="Кто проведёт собрание" /><Main><Note kind="neg">Не удалось загрузить результаты опроса.</Note></Main></Screen>;
  const canContinue = initiativeState.data.is_initiator && initiativeState.data.stage === 'poll' && pollState.data.demand_reached;
  const pollClosed = Boolean(initiativeState.data.poll_ends_at && new Date(initiativeState.data.poll_ends_at) <= new Date());
  return <Screen>
    <Header title="Кто проведёт собрание" />
    <Main className="template-main">
      <div className="template-intro"><MaxTypography.Label className="template-eyebrow" variant="medium-strong">ПОСЛЕ ОПРОСА</MaxTypography.Label><MaxTypography.Headline variant="medium">Выберите путь к собранию</MaxTypography.Headline><MaxTypography.Text variant="body" color="secondary">Поддержка «за»: {fmtM2(parseM2(pollState.data.for_m2) ?? 0)}. Для требования нужно {fmtM2(parseM2(pollState.data.demand_m2) ?? 0)}.</MaxTypography.Text></div>
      <Card className="card template-section"><Status kind="acc">Доступно</Status><MaxTypography.Headline variant="small">Через управляющую компанию</MaxTypography.Headline><MaxTypography.Text variant="body" color="secondary">Вы создаёте требование. После его передачи УК организует собрание.</MaxTypography.Text></Card>
      <Card className="card template-section"><Status kind="acc">Доступно</Status><MaxTypography.Headline variant="small">Провести самостоятельно</MaxTypography.Headline><MaxTypography.Text variant="body" color="secondary">Вы сами организуете собрание, выбираете ответственных и ведёте учёт бюллетеней.</MaxTypography.Text></Card>
      {!initiativeState.data.is_initiator && <Note kind="info">Выбрать путь может инициатор.</Note>}
      {!pollState.data.demand_reached && <Note kind="info">Поддержки пока недостаточно для требования.</Note>}
      {!pollClosed && <Note kind="info">Самостоятельное проведение можно выбрать после окончания опроса.</Note>}
      {error !== null && <ErrorNote error={error} />}
    </Main>
    <Foot>
      <Btn onClick={() => navigate(`/initiatives/${id}/demand/new`)} disabled={!canContinue}>Продолжить через УК</Btn>
      <Btn kind="secondary" disabled={!canContinue || !pollClosed || busy} onClick={async () => {
        setBusy(true); setError(null);
        try { await selectPathB(id); navigate(`/initiatives/${id}/meeting/new?path=B`); }
        catch (cause) { setError(cause); setBusy(false); }
      }}>{busy ? 'Открываем…' : 'Провести самостоятельно'}</Btn>
    </Foot>
  </Screen>;
}

export function DemandCreatePage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const initiativeState = useApi(() => id ? fetchInitiative(id) : Promise.reject(new Error('Инициатива не выбрана')), [id]);
  const pollState = useApi(() => id ? fetchPollProgress(id) : Promise.reject(new Error('Опрос не выбран')), [id]);
  const [channel, setChannel] = useState<DemandRecord['channel']>('paper');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);

  if (initiativeState.loading || pollState.loading) return <Screen><Header title="Требование в УК" /><Main><Card className="card">Загружаем данные опроса…</Card></Main></Screen>;
  if (initiativeState.error || pollState.error || !initiativeState.data || !pollState.data || !id) return <Screen><Header title="Требование в УК" /><Main><Note kind="neg">Не удалось загрузить инициативу и результаты опроса.</Note></Main></Screen>;

  const initiative = initiativeState.data;
  const poll = pollState.data;
  const canCreate = initiative.is_initiator && initiative.stage === 'poll' && poll.demand_reached;
  const submit = async () => {
    if (!canCreate || busy) return;
    setBusy(true);
    setError(null);
    try {
      const demand = await createDemand(id, channel);
      navigate(`/demands/${demand.id}`, { replace: true });
    } catch (cause) {
      setError(cause);
      setBusy(false);
    }
  };

  return <Screen>
    <Header title="Требование в УК" />
    <Main className="template-main">
      <div className="template-intro">
        <MaxTypography.Label className="template-eyebrow" variant="medium-strong">ПОСЛЕ ОПРОСА</MaxTypography.Label>
        <MaxTypography.Headline className="template-title" variant="medium">Попросить УК провести собрание</MaxTypography.Headline>
        <MaxTypography.Text variant="body" color="secondary">Требование будет связано с инициативой «{initiative.title}».</MaxTypography.Text>
      </div>
      <Card className="card template-section">
        <div className="between"><MaxTypography.Headline variant="small">Поддержка собственников</MaxTypography.Headline><Badge kind="poll" /></div>
        <Kv k="За инициативу">{fmtM2(parseM2(poll.for_m2) ?? 0)}</Kv>
        <Kv k="Порог 10%">{fmtM2(parseM2(poll.demand_m2) ?? 0)}</Kv>
        <Status kind={poll.demand_reached ? 'ok' : 'bad'}>{poll.demand_reached ? 'Порог достигнут' : 'Поддержки пока недостаточно'}</Status>
      </Card>
      <Card className="card template-section">
        <MaxTypography.Headline variant="small">Форма будущего собрания</MaxTypography.Headline>
        <div className="template-choice-group" role="group" aria-label="Форма собрания">
          <Opt on={channel === 'paper'} onClick={() => setChannel('paper')} title="Бумажные бюллетени" caption="Собственники заполняют бюллетени; результаты вносят после приёма." />
          <Opt on={channel === 'gosuslugi_dom'} onClick={() => setChannel('gosuslugi_dom')} title="Госуслуги.Дом" caption="Электронное голосование через ГИС ЖКХ." />
        </div>
      </Card>
      <Note kind="info">После создания скачайте PDF, соберите подписи и отметьте дату передачи в УК.</Note>
      {!initiative.is_initiator && <Note kind="info">Создать требование может инициатор.</Note>}
      <ErrorNote error={error} />
    </Main>
    <Foot><Btn onClick={submit} disabled={!canCreate || busy}>{busy ? 'Создаём…' : 'Создать требование'}</Btn></Foot>
  </Screen>;
}

export function DemandPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const demandState = useApi(() => id ? fetchDemand(id) : Promise.reject(new Error('Требование не выбрано')), [id]);
  const meState = useApi(() => fetchMe());
  const initiativeId = demandState.data?.initiative_id;
  const initiativeState = useApi(() => initiativeId ? fetchInitiative(initiativeId) : Promise.resolve(null), [initiativeId]);
  const [updated, setUpdated] = useState<DemandRecord | null>(null);
  const [deliveryDate, setDeliveryDate] = useState(todayInputDate);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const demand = updated ?? demandState.data;

  if (demandState.loading) return <Screen><Header title="Требование в УК" /><Main><Card className="card">Загружаем требование…</Card></Main></Screen>;
  if (demandState.error || !demand || !id) return <Screen><Header title="Требование в УК" /><Main><Note kind="neg">Не удалось загрузить требование.</Note></Main></Screen>;

  const download = async () => {
    setBusy(true);
    setError(null);
    try { await downloadDemandPDF(id); } catch (cause) { setError(cause); }
    setBusy(false);
  };
  const markDelivered = async () => {
    if (!deliveryDate || busy) return;
    setBusy(true);
    setError(null);
    try {
      const deliveredAt = new Date(`${deliveryDate}T12:00:00`).toISOString();
      setUpdated(await markDemandDelivered(id, deliveredAt));
    } catch (cause) { setError(cause); }
    setBusy(false);
  };

  return <Screen>
    <Header title="Требование в УК" onNav={() => navigate(`/initiatives/${demand.initiative_id}`)} />
    <Main className="template-main">
      <div className="template-intro">
        <MaxTypography.Label className="template-eyebrow" variant="medium-strong">ПУТЬ ЧЕРЕЗ УК</MaxTypography.Label>
        <MaxTypography.Headline variant="medium">Требование о собрании</MaxTypography.Headline>
        <Status kind={demand.overdue ? 'bad' : demand.status === 'delivered' ? 'ok' : 'acc'}>{demand.overdue ? 'Срок истёк' : demand.status === 'delivered' ? 'Передано в УК' : 'Ожидает передачи'}</Status>
      </div>
      <Card className="card template-section">
        <div className="between"><MaxTypography.Headline variant="small">Документ</MaxTypography.Headline><Badge kind="fact" /></div>
        <Kv k="Поддержка">{fmtM2(parseM2(demand.support_m2) ?? 0)}</Kv>
        <Kv k="Форма собрания">{demand.channel === 'paper' ? 'Бумажные бюллетени' : 'Госуслуги.Дом'}</Kv>
        <Kv k="Создано">{dateText(demand.created_at)}</Kv>
        <Btn kind="secondary" icon="download" onClick={download} disabled={busy}>Скачать PDF</Btn>
      </Card>
      {demand.status === 'draft' && initiativeState.data?.is_initiator ? <>
        <Note kind="info">Передайте подписанное требование в УК и сохраните копию с отметкой о приёме.</Note>
        <Card className="card template-section">
          <MaxTypography.Headline variant="small">Дата передачи в УК</MaxTypography.Headline>
          <UiInput type="date" value={deliveryDate} onChange={(event) => setDeliveryDate(event.target.value)} max={todayInputDate()} />
        </Card>
      </> : demand.status === 'delivered' ? <Card className="card template-section">
        <MaxTypography.Headline variant="small">Срок проведения</MaxTypography.Headline>
        <Kv k="Передано">{dateText(demand.delivered_at)}</Kv>
        <Kv k="Срок для УК">{dateText(demand.uk_due_at)}</Kv>
        {demand.overdue && <Note kind="neg">Срок истёк. Свяжитесь с УК по поводу проведения собрания.</Note>}
      </Card> : <Note kind="info">Требование готовится к передаче инициатором.</Note>}
      {demand.status === 'delivered' && Boolean(meState.data?.orgs.length) && <Btn kind="secondary" onClick={() => navigate(`/initiatives/${demand.initiative_id}/meeting/new`)}>Создать собрание</Btn>}
      <ErrorNote error={error} />
    </Main>
    {demand.status === 'draft' && initiativeState.data?.is_initiator && <Foot><Btn onClick={markDelivered} disabled={busy || !deliveryDate}>{busy ? 'Сохраняем…' : 'Отметить передачу'}</Btn></Foot>}
  </Screen>;
}
