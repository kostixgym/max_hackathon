import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { useParams, useNavigate, useSearchParams, useLocation } from 'react-router-dom';
import { ProgressM2 } from '../../components/ProgressM2';
import { Apt, Badge, Btn, Chip, Foot, Header, IconButton, Kv, Main, Note, Screen, Seg, StateHead, Status, UiButton, UiInput, UiList, Card } from '../../components/ui';
import { Icon } from '../../components/Icon';
import { fmtM2 } from '../../lib/format';
import { useApi } from '../../hooks/useApi';
import {
  fetchMeeting,
  fetchMeetingTracker,
  receiveBallot,
  recordBallotDecisions,
  fetchMeetingResultPreview,
  finalizeMeeting,
  demoFillBallots,
  demoFinishVoting,
  parseM2,
  type MeetingFinal,
} from '../../lib/api';

function AgendaCard({ items }: { items: { position: number; text: string; majority_rule: string }[] }) {
  return (
    <Card className="card" style={{ gap: 0 }}>
      <div className="between" style={{ paddingBottom: 8 }}>
        <MaxTypography.Headline className="h3" variant="small">Повестка</MaxTypography.Headline>
      </div>
      {items.map((a, i) => (
        <div key={a.position} className="between" style={{ padding: i === items.length - 1 ? '10px 0 0' : '10px 0', borderTop: '1px solid var(--line)' }}>
          <span>{a.text}</span>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">{a.majority_rule}</MaxTypography.Text>
        </div>
      ))}
    </Card>
  );
}

function getStatusLabel(status: string): string {
  switch (status) {
    case 'notice': return 'Уведомление';
    case 'voting': return 'Идёт голосование';
    case 'counting': return 'Подсчёт';
    case 'finalized': return 'Завершено';
    default: return status;
  }
}

function getFormLabel(form: string): string {
  switch (form) {
    case 'gis_electronic': return 'Электронное, Госуслуги.Дом';
    case 'paper_absentee': return 'Заочное на бумаге';
    default: return form;
  }
}

// 12. Карточка собрания
export function Meeting() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [demoBusy, setDemoBusy] = useState(false);
  const meetingState = useApi(() => (id ? fetchMeeting(id) : Promise.reject('No ID')), [id]);

  if (meetingState.loading) return <Screen><Header title="Собрание" /><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text></Card></Main></Screen>;
  if (meetingState.error || !meetingState.data || !id) return <Screen><Header title="Собрание" /><Main><Card className="card"><Note kind="neg">Не удалось загрузить собрание.</Note></Card></Main></Screen>;

  const meeting = meetingState.data;
  const runDemoAction = async (action: (meetingId: string) => Promise<unknown>) => {
    if (!id || demoBusy) return;
    setDemoBusy(true);
    try {
      await action(id);
      window.location.reload();
    } catch (error) {
      window.alert(error instanceof Error ? error.message : 'Демо-действие недоступно для этого собрания');
      setDemoBusy(false);
    }
  };
  const participantsM2 = parseM2(meeting.progress.participants_m2) || 0;
  const totalM2 = parseM2(meeting.progress.total_m2) || 0;
  const meetingThresholds = {
    demandM2: totalM2 / 10,
    quorumAboveM2: parseM2(meeting.progress.quorum_above_m2) ?? 0,
    twoThirdsM2: totalM2 * 2 / 3,
  };

  const votingEndsAt = new Date(meeting.voting_ends_at).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' });
  const daysLeft = Math.ceil((new Date(meeting.voting_ends_at).getTime() - Date.now()) / (1000 * 60 * 60 * 24));

  return (
    <Screen>
      <Header title="Собрание" />
      <Main style={{ gap: 14 }}>
        <div className="col" style={{ gap: 8, padding: '0 4px' }}>
          <Status kind="acc" style={{ alignSelf: 'flex-start' }}>{getStatusLabel(meeting.status)}</Status>
          <MaxTypography.Headline className="h1" variant="large-strong">{meeting.title}</MaxTypography.Headline>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">Собрание №{meeting.attempt} · {meeting.house.address}</MaxTypography.Text>
        </div>

        <Card className="card" style={{ gap: 10 }}>
          <div className="between">
            <MaxTypography.Headline className="h3" variant="small">Даты</MaxTypography.Headline>
            <Badge kind="fact" />
          </div>
          <Kv k="Форма">{getFormLabel(meeting.form)}</Kv>
          <Kv k="Голосование">
            {new Date(meeting.voting_starts_at).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' })} – {votingEndsAt}
          </Kv>
          {daysLeft > 0 && meeting.status === 'voting' && <Kv k="Осталось">{daysLeft} дней</Kv>}
        </Card>

        <Card className="card" style={{ gap: 14 }}>
          <ProgressM2 yes={participantsM2} voted={totalM2} totalM2={totalM2} thresholds={meetingThresholds} avgAreaM2={totalM2 / Math.max(meeting.progress.ballots_total, 1)} title="Учтено голосов" badge="calc" lines="quorum" yesLabel="проголосовали" />
          <div className="hr" />
          <div className="between">
            <MaxTypography.Text className="t2" variant="body" color="secondary">Бюллетени получены</MaxTypography.Text>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">{meeting.progress.ballots_received} из {meeting.progress.ballots_total}</MaxTypography.Text>
          </div>
        </Card>

        <AgendaCard items={meeting.agenda_items} />

        {meeting.is_admin && (
          <Card className="card" style={{ gap: 10 }}>
            <div className="between">
              <MaxTypography.Headline className="h3" variant="small">Администратор</MaxTypography.Headline>
              <Badge kind="fact" />
            </div>
            <Kv k="Председатель">{meeting.chair.masked_name}</Kv>
            <Kv k="Секретарь">{meeting.secretary.masked_name}</Kv>
          </Card>
        )}
        {meeting.is_admin && meeting.status === 'voting' && <Card className="card" style={{ gap: 8 }}><MaxTypography.Headline className="h3" variant="small">Демо-проверка</MaxTypography.Headline><MaxTypography.Text className="cap" variant="detail" color="secondary">Действует только для собраний демо-дома.</MaxTypography.Text><Btn kind="text" onClick={() => runDemoAction(demoFillBallots)} disabled={demoBusy}>{demoBusy ? 'Выполняется…' : 'Заполнить демо-бюллетени'}</Btn><Btn kind="text" onClick={() => runDemoAction(demoFinishVoting)} disabled={demoBusy}>Завершить демо-голосование</Btn></Card>}
      </Main>
      <Foot>
        {meeting.is_admin && (meeting.status === 'voting' || meeting.status === 'counting') && (
          <>
            <Btn onClick={() => navigate(`/meetings/${id}/tracker`)}>Трекер квартир</Btn>
            <Btn kind="text" onClick={() => navigate(`/meetings/${id}/result-preview`)}>Предпросмотр итога</Btn>
          </>
        )}
        {meeting.status === 'finalized' && (
          <Note kind="info">Итог зафиксирован. Формирование PDF-протокола пока не поддержано сервером.</Note>
        )}
      </Foot>
    </Screen>
  );
}

function TrackerTabs({ id, walk, flatCount, walkCount }: { id: string; walk: boolean; flatCount: number; walkCount: number }) {
  const navigate = useNavigate();
  return (
    <div className="row">
      <Chip on={!walk} onClick={() => navigate(`/meetings/${id}/tracker`, { replace: true })}>Все · {flatCount}</Chip>
      <Chip on={walk} onClick={() => navigate(`/meetings/${id}/walk`, { replace: true })}>Список обхода · {walkCount}</Chip>
    </div>
  );
}

// 13. Трекер квартир
export function Tracker() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const trackerState = useApi(() => (id ? fetchMeetingTracker(id) : Promise.reject('No ID')), [id]);

  if (trackerState.loading) return <Screen><Header title="Трекер квартир" /><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text></Card></Main></Screen>;
  if (trackerState.error || !trackerState.data || !id) return <Screen><Header title="Трекер квартир" /><Main><Card className="card"><Note kind="neg">Не удалось загрузить трекер.</Note></Card></Main></Screen>;

  const tracker = trackerState.data;
  const statusLabels: Record<string, { kind: 'ok' | 'paper' | 'said' | 'bad' | 'none'; label: string; mark: string }> = {
    counted: { kind: 'ok', label: 'Учтён', mark: '✓' },
    received: { kind: 'paper', label: 'Получен', mark: '▢' },
    pending: { kind: 'none', label: 'Не получен', mark: '·' },
  };

  const walkCount = tracker.ballots.filter(b => b.status === 'pending').length;

  return (
    <Screen>
      <Header title="Трекер квартир" right={<IconButton icon="qr" label="Приём бюллетеня" onClick={() => navigate(`/meetings/${id}/receive`)} />} />
      <Main style={{ gap: 12 }}>
        <TrackerTabs id={id} walk={false} flatCount={tracker.summary.ballots_total} walkCount={walkCount} />
        <Card className="card" style={{ gap: 10, padding: '14px 16px' }}>
          <div className="between">
            <MaxTypography.Headline className="h3" variant="small">Сводка</MaxTypography.Headline>
            <Badge kind="calc" />
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: '8px 12px' }}>
            <span className="row"><Status kind="ok">Учтено</Status><b className="num">{tracker.summary.counted}</b></span>
            <span className="row"><Status kind="paper">Получено</Status><b className="num">{tracker.summary.received}</b></span>
            <span className="row"><Status kind="none">Не получено</Status><b className="num">{tracker.summary.ballots_total - tracker.summary.received}</b></span>
          </div>
        </Card>

        <UiList>
          {tracker.ballots.map((ballot) => {
            const s = statusLabels[ballot.status] || statusLabels.pending;
            return (
              <div key={ballot.id} className="cell" style={{ minHeight: 60 }} onClick={() => navigate(`/meetings/${id}/receive?ballot=${ballot.id}`)}>
                <div className="grow">
                  <span style={{ fontWeight: 600 }}>Кв. {ballot.premise_number} · {fmtM2(parseM2(ballot.weight_m2) || 0)}</span>
                  <MaxTypography.Text className="cap" variant="detail" color="secondary">{ballot.entrance && `Подъезд ${ballot.entrance} · `}{ballot.owner_masked_name}</MaxTypography.Text>
                </div>
                <Status kind={s.kind}>{s.label}</Status>
              </div>
            );
          })}
        </UiList>
      </Main>
    </Screen>
  );
}


// 13б. Список обхода
export function WalkList() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const trackerState = useApi(() => (id ? fetchMeetingTracker(id) : Promise.reject('No ID')), [id]);

  if (trackerState.loading) return <Screen><Header title="Трекер квартир" /><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text></Card></Main></Screen>;
  if (trackerState.error || !trackerState.data || !id) return <Screen><Header title="Трекер квартир" /><Main><Card className="card"><Note kind="neg">Не удалось загрузить трекер.</Note></Card></Main></Screen>;

  const tracker = trackerState.data;
  const list = tracker.ballots
    .filter(b => b.status === 'pending')
    .sort((a, b) => (a.entrance || 0) - (b.entrance || 0) || Number(a.premise_number) - Number(b.premise_number));

  return (
    <Screen>
      <Header title="Трекер квартир" />
      <Main style={{ gap: 12 }}>
        <TrackerTabs id={id} walk walkCount={list.length} flatCount={tracker.summary.ballots_total} />
        <MaxTypography.Text className="t2" style={{ padding: '0 4px' }} variant="body" color="secondary">Кто ещё не голосовал — по подъездам и этажам, чтобы обойти за один раз.</MaxTypography.Text>
        <UiList>
          {list.map((f) => (
            <div key={f.id} className="cell" onClick={() => navigate(`/meetings/${id}/receive?ballot=${f.id}`)}>
              <Apt n={f.premise_number} />
              <span className="grow">
                <span style={{ fontWeight: 600 }}>Кв. {f.premise_number} · {fmtM2(parseM2(f.weight_m2) || 0)}</span>
                <MaxTypography.Text className="cap" variant="detail" color="secondary">{f.entrance && `Подъезд ${f.entrance} · `}{f.owner_masked_name}</MaxTypography.Text>
              </span>
              <Status kind="none">Не получен</Status>
            </div>
          ))}
        </UiList>
      </Main>
    </Screen>
  );
}

// 14. Приём бюллетеня
export function Receive() {
  const { id } = useParams<{ id: string }>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const [number, setNumber] = useState('');
  const [loading, setLoading] = useState(false);
  const [receivedBallotData, setReceivedBallotData] = useState<{ premise: string; name: string; id: string } | null>(null);

  const trackerState = useApi(() => (id ? fetchMeetingTracker(id) : Promise.reject('No ID')), [id]);
  const selectedBallot = searchParams.get('ballot');

  const handleReceive = async (ballotId: string) => {
    if (!id || loading) return;
    setLoading(true);
    try {
      await receiveBallot(id, ballotId);
      const b = trackerState.data?.ballots.find(x => x.id === ballotId);
      if (b) setReceivedBallotData({ premise: b.premise_number, name: b.owner_masked_name, id: b.id });
    } catch (e) {
      alert('Не удалось отметить получение бюллетеня');
    } finally {
      setLoading(false);
    }
  };

  const found = selectedBallot
    ? trackerState.data?.ballots.find((b) => b.id === selectedBallot)
    : number ? trackerState.data?.ballots.find(b => b.premise_number === number) : null;

  return (
    <Screen>
      <Header title="Приём бюллетеня" />
      <Main style={{ gap: 14 }}>
        <UiButton type="button" variant="secondary" className="qr-scan-card" onClick={() => alert('Сканер QR будет доступен в мобильной версии')}>
          <span className="circ" style={{ background: 'var(--acc-soft)', color: 'var(--acc-t)', width: 72, height: 72 }}><Icon name="scan" /></span>
          <MaxTypography.Headline className="h3" variant="small">Сканировать QR</MaxTypography.Headline>
          <MaxTypography.Text className="cap" style={{ textAlign: 'center' }} variant="detail" color="secondary">QR-код напечатан в углу каждого бюллетеня</MaxTypography.Text>
        </UiButton>
        <div className="row" style={{ gap: 12, padding: '0 4px' }}>
          <div className="hr" style={{ flex: 1 }} /><MaxTypography.Text className="cap" variant="detail" color="secondary">или вручную</MaxTypography.Text><div className="hr" style={{ flex: 1 }} />
        </div>
        <div className="row" style={{ gap: 8, alignItems: 'flex-end' }}>
          <label className="field" style={{ flex: 1 }}>
            <MaxTypography.Label className="lbl" variant="large-strong">Номер квартиры</MaxTypography.Label>
            <UiInput inputMode="numeric" value={number} onChange={(e) => { setNumber(e.target.value.replace(/\D/g, '')); setReceivedBallotData(null); }} />
          </label>
        </div>
        {found && !receivedBallotData && (
          <Card className="card" style={{ gap: 10 }}>
            <div className="grow">
              <span style={{ fontWeight: 600 }}>Кв. {found.premise_number} · {found.owner_masked_name}</span>
              <MaxTypography.Text className="cap" variant="detail" color="secondary">Статус: {found.status === 'pending' ? 'Не получен' : 'Уже получен'}</MaxTypography.Text>
            </div>
            {found.status === 'pending' && <Btn small onClick={() => handleReceive(found.id)} disabled={loading}>Принять</Btn>}
          </Card>
        )}
        {receivedBallotData && (
          <div className="note n-pos" style={{ flexDirection: 'column', gap: 8 }} role="status">
            <div className="row" style={{ gap: 10 }}><Icon name="check" style={{ color: 'var(--pos)' }} /><b style={{ fontSize: 18 }}>Бюллетень кв. {receivedBallotData.premise} получен</b></div>
            <MaxTypography.Text className="cap" style={{ color: 'var(--text)' }} variant="detail" color="secondary">{receivedBallotData.name}</MaxTypography.Text>
          </div>
        )}
      </Main>
      <Foot>
        {receivedBallotData && <Btn onClick={() => navigate(`/meetings/${id}/enter/${receivedBallotData.id}`)}>Внести решения</Btn>}
        <Btn kind="text" onClick={() => { setNumber(''); setReceivedBallotData(null); }}>Принять следующий</Btn>
      </Foot>
    </Screen>
  );
}

type Choice = 'for' | 'against' | 'abstain';

// 15. Внесение решений из бумажного бюллетеня
export function EnterDecisions() {
  const { id: meetingId, ballotId } = useParams<{ id: string; ballotId: string }>();
  const navigate = useNavigate();
  const [answers, setAnswers] = useState<Record<string, Choice>>({});
  const [signed, setSigned] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  const meetingState = useApi(() => (meetingId ? fetchMeeting(meetingId) : Promise.reject('No ID')), [meetingId]);
  const trackerState = useApi(() => (meetingId ? fetchMeetingTracker(meetingId) : Promise.reject('No ID')), [meetingId]);

  const meeting = meetingState.data;
  const ballot = trackerState.data?.ballots.find(b => b.id === ballotId);

  const handleSubmit = async () => {
    if (!ballotId || !meeting || submitting) return;
    const decisions = meeting.agenda_items.map(a => ({ agenda_item_id: a.id, choice: answers[a.id] || 'abstain' }));
    setSubmitting(true);
    try {
      await recordBallotDecisions(ballotId, { decisions });
      navigate(`/meetings/${meetingId}/tracker`);
    } catch (e) {
      alert('Не удалось сохранить решения');
    } finally {
      setSubmitting(false);
    }
  };

  if (!meeting || !ballot) return <Screen><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text></Card></Main></Screen>;

  return (
    <Screen>
      <Header title={`Бюллетень кв. ${ballot.premise_number}`} />
      <Main style={{ gap: 12 }}>
        <Card className="card" style={{ flexDirection: 'row', alignItems: 'center', gap: 12, padding: '12px 16px' }}>
          <div className="grow">
            <span style={{ fontWeight: 600 }}>{ballot.owner_masked_name}</span>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">Кв. {ballot.premise_number}</MaxTypography.Text>
          </div>
          <b className="num" style={{ fontSize: 18 }}>{fmtM2(parseM2(ballot.weight_m2) || 0)}</b>
          <Badge kind="calc" style={{ alignSelf: 'center' }} />
        </Card>
        <MaxTypography.Text className="t2" style={{ padding: '0 4px' }} variant="body" color="secondary">Перенесите отметки из бумажного бюллетеня — по одной на вопрос.</MaxTypography.Text>
        {meeting.agenda_items.map((a) => (
          <Card key={a.id} className="card" style={{ gap: 10 }}>
            <span style={{ fontWeight: 600 }}>{a.text}</span>
            <Seg
              value={answers[a.id] || 'abstain'}
              onChange={(v) => setAnswers(prev => ({ ...prev, [a.id]: v as Choice }))}
              options={[{ value: 'for', label: 'За' }, { value: 'against', label: 'Против' }, { value: 'abstain', label: 'Воздержался' }]}
            />
          </Card>
        ))}
        <Card className="card" style={{ gap: 12 }}>
          <UiButton type="button" variant="ghost" size="medium" className="signature-toggle" onClick={() => setSigned((s) => !s)}>
            <span className={'cb' + (signed ? ' on' : '')}>{signed && <Icon name="check" style={{ width: 16, height: 16, strokeWidth: 3 }} />}</span>
            <span>Подпись и дата на месте</span>
          </UiButton>
        </Card>
      </Main>
      <Foot>
        <Btn onClick={handleSubmit} disabled={!signed || submitting}>{submitting ? 'Сохранение...' : 'Сохранить бюллетень'}</Btn>
      </Foot>
    </Screen>
  );
}

// 16. Проверка и фиксация итога
export function ResultPreview() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [submitting, setSubmitting] = useState(false);
  const previewState = useApi(() => (id ? fetchMeetingResultPreview(id) : Promise.reject('No ID')), [id]);

  if (previewState.loading) return <Screen><Header title="Проверка итога" /><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text></Card></Main></Screen>;
  if (previewState.error || !previewState.data || !id) return <Screen><Header title="Проверка итога" /><Main><Card className="card"><Note kind="neg">Не удалось загрузить предпросмотр.</Note></Card></Main></Screen>;

  const res = previewState.data;
  const participantsM2 = parseM2(res.participants_m2) || 0;
  const totalM2 = parseM2(res.total_m2) || 0;
  const percent = totalM2 > 0 ? Math.round((participantsM2 / totalM2) * 100) : 0;

  const handleFinalize = async () => {
    if (!id || submitting) return;
    setSubmitting(true);
    try {
      const final = await finalizeMeeting(id);
      navigate(`/meetings/${id}/result`, { state: { final } });
    } catch (e) {
      alert('Не удалось зафиксировать итог');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Screen>
      <Header title="Проверка итога" />
      <Main style={{ gap: 12 }}>
        <Card className="card" style={{ gap: 12 }}>
          <div className="between">
            <Status kind={res.quorum_reached ? 'ok' : 'bad'}>{res.quorum_reached ? 'Кворум есть' : 'Кворума нет'}</Status>
            <Badge kind="calc" />
          </div>
          <div className="row wrap" style={{ alignItems: 'baseline', gap: 8 }}>
            <MaxTypography.Display className="big">{fmtM2(participantsM2)}</MaxTypography.Display>
            <MaxTypography.Text className="t2" variant="body" color="secondary">участвовали · {percent}% дома</MaxTypography.Text>
          </div>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">Всего в доме {fmtM2(totalM2)}</MaxTypography.Text>
        </Card>

        {res.agenda_results.map((q) => (
          <Card key={q.agenda_item_id} className="card" style={{ gap: 12 }}>
            <div className="between" style={{ alignItems: 'flex-start' }}>
              <MaxTypography.Headline className="h3" variant="small">{q.text}</MaxTypography.Headline>
              <Status kind={q.accepted ? 'ok' : 'bad'}>{q.accepted ? 'Принят' : 'Не принят'}</Status>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 8 }}>
              {[['За', q.for_m2], ['Против', q.against_m2], ['Воздерж.', q.abstain_m2]].map(([k, v]) => (
                <div key={k} className="col" style={{ gap: 0, background: 'var(--fill)', borderRadius: 10, padding: '8px 10px' }}>
                  <MaxTypography.Text className="cap" variant="detail" color="secondary">{k}</MaxTypography.Text>
                  <b className="num">{fmtM2(parseM2(v as string) || 0)}</b>
                </div>
              ))}
            </div>
            <MaxTypography.Text className="cap" variant="detail" color="secondary"><b style={{ color: 'var(--text)' }}>Правило:</b> {q.majority_rule}</MaxTypography.Text>
          </Card>
        ))}
      </Main>
      <Foot>
        <Note kind="neg" icon="lock" style={{ padding: '10px 14px', fontSize: 15, lineHeight: '21px' }}>После фиксации итог нельзя изменить.</Note>
        <Btn onClick={handleFinalize} disabled={submitting}>{submitting ? 'Фиксация...' : 'Зафиксировать итог'}</Btn>
      </Foot>
    </Screen>
  );
}

// 17. Итог собрания
export function Result() {
  const { id } = useParams<{ id: string }>();
  const location = useLocation();
  const meetingState = useApi(() => (id ? fetchMeeting(id) : Promise.reject('No ID')), [id]);

  if (meetingState.loading) return <Screen><Header title="Итог собрания" /><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text></Card></Main></Screen>;
  if (meetingState.error || !meetingState.data || !id) return <Screen><Header title="Итог собрания" /><Main><Card className="card"><Note kind="neg">Не удалось загрузить данные.</Note></Card></Main></Screen>;

  const meeting = meetingState.data;
  const final = (location.state as { final?: MeetingFinal } | null)?.final;
  const percent = parseM2(meeting.progress.total_m2)! > 0 ? Math.round((parseM2(meeting.progress.participants_m2)! / parseM2(meeting.progress.total_m2)!) * 100) : 0;

  return (
    <Screen>
      <Header title="Итог собрания" />
      <Main style={{ gap: 14 }}>
        <Card className="card" style={{ alignItems: 'center', gap: 10, padding: '24px 16px' }}>
          <StateHead icon="check" tone="pos" title={meeting.outcome === 'accepted' ? 'Решение принято' : 'Решение не принято'} text={`Кворум ${percent}% · зафиксировано ${new Date(meeting.finalized_at!).toLocaleDateString('ru-RU')}`} />
          <Badge kind="fact" style={{ alignSelf: 'center' }}>Официальный итог</Badge>
        </Card>

        <UiList>
          {(final?.results ?? meeting.agenda_items.map((q) => ({ agenda_item_id: q.id, text: q.text, accepted: null }))).map((q) => (
            <div key={q.agenda_item_id} className="cell">
              <span className="grow"><span style={{ fontWeight: 600 }}>{q.text}</span></span>
              {typeof q.accepted === 'boolean' && <Status kind={q.accepted ? 'ok' : 'bad'}>{q.accepted ? 'Принят' : 'Не принят'}</Status>}
            </div>
          ))}
        </UiList>

        <Note kind="info" icon="calendar">Итог зафиксирован в системе. Ручная выгрузка PDF-протокола пока недоступна.</Note>
      </Main>
    </Screen>
  );
}
