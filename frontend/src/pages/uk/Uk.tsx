import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { Btn, Cell, Header, Main, Note, Screen, Status, Tile, UiList, Card } from '../../components/ui';
import { useApi } from '../../hooks/useApi';
import { becomeDemoStaff, decideOrgOwnerRequest, fetchMe, fetchOrgDemands, fetchOrgHouses, fetchOrgOwnerRequests, fetchOrgs, getStartParam, parseM2, searchHouses } from '../../lib/api';
import { fmtM2 } from '../../lib/format';

export function UkHouses() {
  const meState = useApi(() => fetchMe());
  const orgsState = useApi(() => fetchOrgs());
  const org = orgsState.data?.orgs[0];
  const housesState = useApi(() => org ? fetchOrgHouses(org.id) : Promise.resolve({ houses: [] }), [org?.id]);
  const houseSlug = meState.data?.house?.slug ?? getStartParam();
  const demoHouseState = useApi(async () => {
    if (houseSlug || org) return null;
    const result = await searchHouses('Демонстрационная');
    return result.houses.find((house) => house.is_demo) ?? null;
  }, [houseSlug, org?.id]);
  const [joining, setJoining] = useState(false);
  const demoSlug = houseSlug ?? demoHouseState.data?.slug ?? null;

  const joinDemo = async () => {
    if (!demoSlug || joining) return;
    setJoining(true);
    try {
      await becomeDemoStaff(demoSlug);
      window.location.reload();
    } catch (error) {
      window.alert(error instanceof Error ? error.message : 'Не удалось открыть кабинет демо-УК');
      setJoining(false);
    }
  };

  if (meState.loading || orgsState.loading || housesState.loading || demoHouseState.loading) return <Screen><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка кабинета…</MaxTypography.Text></Card></Main></Screen>;
  if (meState.error || orgsState.error || housesState.error || demoHouseState.error) return <Screen><Main><Note kind="neg">Не удалось загрузить кабинет УК.</Note></Main></Screen>;

  const orgs = orgsState.data?.orgs ?? [];
  const houses = housesState.data?.houses ?? [];
  return <Screen>
    <div className="col" style={{ gap: 12, padding: '12px 16px 8px' }}>
      <div className="row"><div className="grow"><MaxTypography.Headline className="h2" variant="medium">{org?.name ?? 'Кабинет УК'}</MaxTypography.Headline><MaxTypography.Text className="cap" variant="detail" color="secondary">Дома, доступные вашей организации</MaxTypography.Text></div>{org && <Status kind="mod">Сотрудник УК</Status>}</div>
    </div>
    <Main style={{ gap: 12 }}>
      <div className="between"><MaxTypography.Headline className="h3" variant="small">Дома</MaxTypography.Headline><MaxTypography.Text className="cap" variant="detail" color="secondary">{houses.length}</MaxTypography.Text></div>
      {houses.length > 0 ? <UiList>{houses.map((house) => <Cell key={house.id} lead={<Tile icon="building" />}><span style={{ fontWeight: 600 }}>{house.address}</span><MaxTypography.Text className="cap" variant="detail" color="secondary">{house.region}{house.is_demo ? ' · демо-дом' : ''}</MaxTypography.Text></Cell>)}</UiList> : <Note kind="info">У организации пока нет доступных домов.</Note>}
      {orgs.length === 0 && meState.data?.uk_ids_configured && <Note kind="info">Доступ в кабинет УК выдаётся по MAX ID. Отправьте боту команду /id и передайте номер администратору.</Note>}
      {orgs.length === 0 && !meState.data?.uk_ids_configured && demoSlug && <><Note kind="info">В демо-кабинете появятся заявки, которые жильцы отправили на ручную проверку, и требования, созданные по итогам опросов.</Note><Btn onClick={joinDemo} disabled={joining}>{joining ? 'Подключаем…' : 'Войти в демо-кабинет УК'}</Btn></>}
      {orgs.length === 0 && !meState.data?.uk_ids_configured && !demoSlug && <Note kind="info">Демо-дом не найден. Проверьте, что сервер запущен с `SEED_DEMO=true`.</Note>}
      {orgs.length > 0 && <UiList>
        <Cell to="/uk/demands" chevron lead={<Tile icon="receipt" />}><MaxTypography.Title variant="small-strong">Входящие требования</MaxTypography.Title><MaxTypography.Text variant="detail" color="secondary">От собственников домов УК</MaxTypography.Text></Cell>
        <Cell to="/uk/requests" chevron lead={<Tile icon="personPlus" />}><MaxTypography.Title variant="small-strong">Заявки собственников</MaxTypography.Title><MaxTypography.Text variant="detail" color="secondary">Проверить или отклонить выбор записи из реестра</MaxTypography.Text></Cell>
      </UiList>}
    </Main>
  </Screen>;
}

export function UkDemands() {
  const orgsState = useApi(() => fetchOrgs());
  const orgIds = orgsState.data?.orgs.map((org) => org.id).join(',') ?? '';
  const demandsState = useApi(async () => {
    const ids = orgIds ? orgIds.split(',') : [];
    const lists = await Promise.all(ids.map((id) => fetchOrgDemands(id)));
    return lists.flatMap((list) => list.demands);
  }, [orgIds]);

  if (orgsState.loading || demandsState.loading) return <Screen><Header title="Требования собственников" /><Main><Card className="card">Загружаем требования…</Card></Main></Screen>;
  if (orgsState.error || demandsState.error) return <Screen><Header title="Требования собственников" /><Main><Note kind="neg">Не удалось загрузить требования. Попробуйте обновить страницу.</Note></Main></Screen>;
  const demands = demandsState.data ?? [];
  return <Screen>
    <Header title="Требования собственников" />
    <Main className="template-main">
      <div className="template-intro"><MaxTypography.Label className="template-eyebrow" variant="medium-strong">КАБИНЕТ УК</MaxTypography.Label><MaxTypography.Headline variant="medium">Входящие требования</MaxTypography.Headline><MaxTypography.Text variant="body" color="secondary">{demands.length} по вашим домам</MaxTypography.Text></div>
      {demands.length === 0 ? <Note kind="info">Здесь появятся требования, когда собственники наберут порог голосов в инициативе и передадут её в УК. Пока переданных требований нет.</Note> : <UiList className="template-list">{demands.map((demand) => <Cell key={demand.id} to={`/demands/${demand.id}`} chevron lead={<Tile icon="receipt" />}>
        <MaxTypography.Title variant="small-strong">{demand.initiative_title}</MaxTypography.Title>
        <MaxTypography.Text variant="detail" color="secondary">{demand.house.address}</MaxTypography.Text>
        <MaxTypography.Text variant="detail" color="secondary">{demand.status === 'delivered' && demand.delivered_at ? `Передано ${new Date(demand.delivered_at).toLocaleDateString('ru-RU')}` : 'Готовится к передаче'} · поддержка {fmtM2(parseM2(demand.support_m2) ?? 0)}</MaxTypography.Text>
        <Status kind={demand.overdue ? 'bad' : demand.status === 'delivered' ? 'ok' : 'acc'}>{demand.overdue ? 'Просрочено' : demand.status === 'delivered' ? 'Передано' : 'Черновик'}</Status>
      </Cell>)}</UiList>}
    </Main>
  </Screen>;
}

export function UkCreateMeeting() {
  const orgsState = useApi(() => fetchOrgs());
  const orgIds = orgsState.data?.orgs.map((org) => org.id).join(',') ?? '';
  const demandsState = useApi(async () => (await Promise.all((orgIds ? orgIds.split(',') : []).map(fetchOrgDemands))).flatMap((item) => item.demands), [orgIds]);
  if (orgsState.loading || demandsState.loading) return <Screen><Header title="Новое собрание" /><Main><Card className="card">Загружаем требования…</Card></Main></Screen>;
  if (orgsState.error || demandsState.error) return <Screen><Header title="Новое собрание" /><Main><Note kind="neg">Не удалось загрузить требования.</Note></Main></Screen>;
  const ready = (demandsState.data ?? []).filter((demand) => demand.status === 'delivered');
  return <Screen><Header title="Новое собрание" /><Main className="template-main">
    <div className="template-intro"><MaxTypography.Label className="template-eyebrow" variant="medium-strong">КАБИНЕТ УК</MaxTypography.Label><MaxTypography.Headline variant="medium">Выберите требование</MaxTypography.Headline><MaxTypography.Text variant="body" color="secondary">Собрание создаётся по инициативе после передачи требования в УК.</MaxTypography.Text></div>
    {ready.length ? <UiList className="template-list">{ready.map((demand) => <Cell key={demand.id} to={`/initiatives/${demand.initiative_id}/meeting/new`} chevron lead={<Tile icon="calendar" />}><MaxTypography.Title variant="small-strong">{demand.initiative_title}</MaxTypography.Title><MaxTypography.Text variant="detail" color="secondary">{demand.house.address}</MaxTypography.Text></Cell>)}</UiList> : <Note kind="info">Переданных требований пока нет.</Note>}
    <Btn kind="secondary" to="/uk/demands">Все требования</Btn>
  </Main></Screen>;
}

export function UkRequests() {
  const orgsState = useApi(() => fetchOrgs());
  const orgIds = orgsState.data?.orgs.map((org) => org.id).join(',') ?? '';
  const requestsState = useApi(async () => {
    const ids = orgIds ? orgIds.split(',') : [];
    const lists = await Promise.all(ids.map(async (orgId) => ({ orgId, requests: (await fetchOrgOwnerRequests(orgId)).requests })));
    return lists.flatMap(({ orgId, requests }) => requests.map((request) => ({ ...request, orgId })));
  }, [orgIds]);
  const [busyId, setBusyId] = useState('');
  const [error, setError] = useState('');
  const decide = async (orgId: string, membershipId: string, approve: boolean) => {
    if (busyId) return;
    setBusyId(membershipId); setError('');
    try { await decideOrgOwnerRequest(orgId, membershipId, approve); window.location.reload(); }
    catch (cause) { setError(cause instanceof Error ? cause.message : 'Не удалось обработать заявку'); setBusyId(''); }
  };
  if (orgsState.loading || requestsState.loading) return <Screen><Header title="Заявки собственников" /><Main><Card className="card">Загружаем заявки…</Card></Main></Screen>;
  if (orgsState.error || requestsState.error) return <Screen><Header title="Заявки собственников" /><Main><Note kind="neg">Не удалось загрузить заявки. Попробуйте обновить страницу.</Note></Main></Screen>;
  const requests = requestsState.data ?? [];
  return <Screen><Header title="Заявки собственников" /><Main className="template-main">
    <div className="template-intro"><MaxTypography.Label className="template-eyebrow" variant="medium-strong">КАБИНЕТ УК</MaxTypography.Label><MaxTypography.Headline variant="medium">Подтверждение собственников</MaxTypography.Headline><MaxTypography.Text variant="body" color="secondary">{requests.length} заявок ожидают проверки по реестру</MaxTypography.Text></div>
    {error && <Note kind="neg">{error}</Note>}
    {requests.length === 0 ? <Note kind="info">Заявок пока нет. Они появятся, когда жилец выберет квартиру и отправит запись собственника из реестра на проверку УК.</Note> : requests.map((request) => {
      return <Card className="card" key={request.membership_id} style={{ gap: 10 }}>
        <div className="between"><MaxTypography.Headline variant="small">Квартира {request.owner.premise?.number ?? '—'}</MaxTypography.Headline><Status kind="acc">Ожидает проверки</Status></div>
        <MaxTypography.Text variant="body">Заявитель указал: {request.owner.masked_name}</MaxTypography.Text>
        <MaxTypography.Text variant="detail" color="secondary">Доля {request.owner.share.numerator}/{request.owner.share.denominator} · вес {fmtM2(parseM2(request.owner.weight_m2) ?? 0)}</MaxTypography.Text>
        <div className="row" style={{ gap: 8 }}><Btn onClick={() => decide(request.orgId, request.membership_id, true)} disabled={busyId === request.membership_id}>{busyId === request.membership_id ? 'Сохраняем…' : 'Подтвердить'}</Btn><Btn kind="secondary" onClick={() => decide(request.orgId, request.membership_id, false)} disabled={busyId === request.membership_id}>Отклонить</Btn></div>
      </Card>;
    })}
    <Btn kind="secondary" to="/uk">К домам УК</Btn>
  </Main></Screen>;
}
