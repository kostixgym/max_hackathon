import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Btn, Card, Foot, Header, Main, Note, Opt, Screen } from '../../components/ui';
import { useApi } from '../../hooks/useApi';
import { createMeeting, fetchInitiative, fetchInitiativeDemand, fetchMeetingOfficerCandidates, fetchOrgHouses, fetchOrgs, type CreateMeetingInput } from '../../lib/api';

function dateTimeValue(offsetDays: number): string {
  const date = new Date(Date.now() + offsetDays * 24 * 60 * 60 * 1000);
  date.setHours(date.getHours() + 1, 0, 0, 0);
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}T${String(date.getHours()).padStart(2, '0')}:00`;
}

export function MeetingCreatePage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const initiativeState = useApi(() => id ? fetchInitiative(id) : Promise.reject(new Error('Инициатива не выбрана')), [id]);
  const demandState = useApi(() => id ? fetchInitiativeDemand(id) : Promise.reject(new Error('Требование не выбрано')), [id]);
  const orgsState = useApi(() => fetchOrgs());
  const orgIds = orgsState.data?.orgs.map((org) => org.id).join(',') ?? '';
  const housesState = useApi(async () => (await Promise.all((orgIds ? orgIds.split(',') : []).map(fetchOrgHouses))).flatMap((item) => item.houses), [orgIds]);
  const house = housesState.data?.find((item) => item.id === initiativeState.data?.house_id);
  const candidatesState = useApi(() => house ? fetchMeetingOfficerCandidates(house.slug) : Promise.resolve({ owners: [] }), [house?.slug]);

  const [form, setForm] = useState<CreateMeetingInput['form']>('paper_absentee');
  const [notice, setNotice] = useState(dateTimeValue(0));
  const [starts, setStarts] = useState(dateTimeValue(11));
  const [ends, setEnds] = useState(dateTimeValue(14));
  const [chair, setChair] = useState('');
  const [secretary, setSecretary] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (demandState.data) setForm(demandState.data.channel === 'gosuslugi_dom' ? 'gis_electronic' : 'paper_absentee');
  }, [demandState.data]);

  if (initiativeState.loading || demandState.loading || orgsState.loading || housesState.loading || candidatesState.loading) return <Screen><Header title="Новое собрание" /><Main><Card className="card">Загружаем данные дома…</Card></Main></Screen>;
  if (initiativeState.error || demandState.error || orgsState.error || housesState.error || candidatesState.error || !initiativeState.data || !demandState.data || !id) return <Screen><Header title="Новое собрание" /><Main><Note kind="neg">Не удалось загрузить данные для собрания.</Note></Main></Screen>;
  if (!house) return <Screen><Header title="Новое собрание" /><Main><Note kind="info">Выберите дом вашей управляющей организации с инициативой на этапе требования.</Note></Main></Screen>;

  const candidates = candidatesState.data?.owners ?? [];
  const isDemo = house.is_demo;
  const validDates = notice && starts && ends && new Date(starts) >= new Date(notice) && new Date(ends) > new Date(starts)
    && (isDemo || new Date(starts).getTime() - new Date(notice).getTime() >= 10 * 24 * 60 * 60 * 1000);
  const canSubmit = initiativeState.data.stage === 'demand' && demandState.data.status === 'delivered' && Boolean(validDates && chair && secretary && chair !== secretary);

  const submit = async () => {
    if (!canSubmit || busy) return;
    setBusy(true);
    setError('');
    try {
      const meeting = await createMeeting(id, {
        form,
        notice_at: new Date(notice).toISOString(),
        voting_starts_at: new Date(starts).toISOString(),
        voting_ends_at: new Date(ends).toISOString(),
        chair_owner_id: chair,
        secretary_owner_id: secretary,
      });
      navigate(`/meetings/${meeting.id}`, { replace: true });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось создать собрание');
      setBusy(false);
    }
  };

  return <Screen>
    <Header title="Новое собрание" />
    <Main className="template-main">
      <div className="template-intro"><MaxTypography.Label className="template-eyebrow" variant="medium-strong">КАБИНЕТ УК</MaxTypography.Label><MaxTypography.Headline variant="medium">Провести собрание</MaxTypography.Headline><MaxTypography.Text variant="body" color="secondary">{initiativeState.data.title} · {house.address}</MaxTypography.Text></div>
      <Card className="card template-section">
        <div className="template-section-heading"><span className="template-section-number">1</span><MaxTypography.Headline variant="small">Форма собрания</MaxTypography.Headline></div>
        <div className="template-choice-group" role="group" aria-label="Форма собрания">
          <Opt on={form === 'paper_absentee'} onClick={() => setForm('paper_absentee')} title="Заочное, бумажные бюллетени" />
          <Opt on={form === 'gis_electronic'} onClick={() => setForm('gis_electronic')} title="Электронное, Госуслуги.Дом" />
        </div>
      </Card>
      <Card className="card template-section">
        <div className="template-section-heading"><span className="template-section-number">2</span><MaxTypography.Headline variant="small">Даты</MaxTypography.Headline></div>
        <label className="field"><MaxTypography.Label variant="large-strong">Уведомление</MaxTypography.Label><input className="template-textarea template-date-input" type="datetime-local" value={notice} onChange={(event) => setNotice(event.target.value)} /></label>
        <label className="field"><MaxTypography.Label variant="large-strong">Начало голосования</MaxTypography.Label><input className="template-textarea template-date-input" type="datetime-local" value={starts} onChange={(event) => setStarts(event.target.value)} /></label>
        <label className="field"><MaxTypography.Label variant="large-strong">Конец голосования</MaxTypography.Label><input className="template-textarea template-date-input" type="datetime-local" value={ends} onChange={(event) => setEnds(event.target.value)} /></label>
        {!isDemo && <MaxTypography.Text variant="detail" color="secondary">Между уведомлением и началом голосования должно пройти не меньше 10 дней.</MaxTypography.Text>}
        {!validDates && <Note kind="neg">Проверьте даты собрания и срок уведомления.</Note>}
      </Card>
      <Card className="card template-section">
        <div className="template-section-heading"><span className="template-section-number">3</span><MaxTypography.Headline variant="small">Ответственные</MaxTypography.Headline></div>
        {candidates.length === 0 ? <Note kind="info">В доме нет доступных кандидатов.</Note> : <>
          <label className="field"><MaxTypography.Label variant="large-strong">Председатель</MaxTypography.Label><select className="template-textarea template-date-input" value={chair} onChange={(event) => setChair(event.target.value)}><option value="">Выберите собственника</option>{candidates.map((owner) => <option key={owner.id} value={owner.id}>{owner.masked_name} · кв. {owner.premise?.number ?? '—'}</option>)}</select></label>
          <label className="field"><MaxTypography.Label variant="large-strong">Секретарь</MaxTypography.Label><select className="template-textarea template-date-input" value={secretary} onChange={(event) => setSecretary(event.target.value)}><option value="">Выберите собственника</option>{candidates.map((owner) => <option key={owner.id} value={owner.id}>{owner.masked_name} · кв. {owner.premise?.number ?? '—'}</option>)}</select></label>
          {chair && chair === secretary && <Note kind="neg">Председатель и секретарь должны быть разными собственниками.</Note>}
        </>}
      </Card>
      {initiativeState.data.stage !== 'demand' && <Note kind="info">Собрание можно создать, когда инициатива перешла на этап требования.</Note>}
      {demandState.data.status !== 'delivered' && <Note kind="info">Сначала отметьте передачу требования в УК.</Note>}
      {error && <Note kind="neg">{error}</Note>}
    </Main>
    <Foot><Btn onClick={submit} disabled={!canSubmit || busy}>{busy ? 'Создаём…' : 'Создать собрание'}</Btn></Foot>
  </Screen>;
}
