import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { Btn, Cell, Header, Main, Note, Screen, Status, Tile, UiList, Card } from '../../components/ui';
import { useApi } from '../../hooks/useApi';
import { decideOrgOwnerRequest, fetchOrgDemands, fetchOrgHouses, fetchOrgOwnerRequests, fetchOrgs, parseM2 } from '../../lib/api';
import { fmtM2 } from '../../lib/format';

export function UkHouses() {
  const orgsState = useApi(() => fetchOrgs());
  const orgs = orgsState.data?.orgs ?? [];
  const orgIds = orgs.map((item) => item.id).join(',');
  const housesState = useApi(async () => ({ houses: (await Promise.all(orgs.map(async (item) => ({
    orgName: item.name,
    houses: (await fetchOrgHouses(item.id)).houses,
  })))).flatMap(({ orgName, houses }) => houses.map((house) => ({ ...house, orgName }))) }), [orgIds]);
  if (orgsState.loading || housesState.loading) return <Screen><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка кабинета…</MaxTypography.Text></Card></Main></Screen>;
  if (orgsState.error || housesState.error) return <Screen><Main><Note kind="neg">Не удалось загрузить кабинет УК.</Note></Main></Screen>;

  const houses = housesState.data?.houses ?? [];
  return <Screen>
    <div className="col" style={{ gap: 12, padding: '12px 16px 8px' }}>
      <div className="row"><div className="grow"><MaxTypography.Headline className="h2" variant="medium">Кабинет УК</MaxTypography.Headline><MaxTypography.Text className="cap" variant="detail" color="secondary">Дома ваших организаций</MaxTypography.Text></div>{orgs.length > 0 && <Status kind="mod">{orgs.length} организаций</Status>}</div>
    </div>
    <Main style={{ gap: 12 }}>
      <div className="between"><MaxTypography.Headline className="h3" variant="small">Дома</MaxTypography.Headline><MaxTypography.Text className="cap" variant="detail" color="secondary">{houses.length}</MaxTypography.Text></div>
      {houses.length > 0 ? <UiList>{houses.map((house) => <Cell key={`${house.orgName}-${house.id}`} lead={<Tile icon="building" />}><span style={{ fontWeight: 600 }}>{house.address}</span><MaxTypography.Text className="cap" variant="detail" color="secondary">{house.region} · {house.orgName}{house.is_demo ? ' · демо-дом' : ''}</MaxTypography.Text></Cell>)}</UiList> : <Note kind="info">У ваших организаций пока нет доступных домов.</Note>}
      {orgs.length === 0 && <Note kind="info">Доступ к кабинету УК выдаёт системный администратор.</Note>}
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
