import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button as MaxButton, CellAction as MaxCellAction, Typography, Typography as MaxTypography } from '@maxhub/max-ui';
import { Apt, Btn, Cell, DemoBadge, Foot, Main, Note, Screen, Status, UiAvatar, UiInput, UiList, Card } from '../components/ui';
import { useApi } from '../hooks/useApi';
import { attachGuest, becomeDemoStaff, confirmDemoMembership, fetchMe, fetchOwnerClaimCandidates, getDisplayUser, searchHouses, submitOwnerClaim, verifyMembershipPhone, type HouseJSON, type MeResponse, type Owner } from '../lib/api';
import { fmtM2Str } from '../lib/format';

function HouseCard({ house, memberships, onOpen }: {
  house: MeResponse['memberships'][number]['house'];
  memberships: MeResponse['memberships'];
  onOpen: () => void;
}) {
  return (
    <Card className="entry-house-panel" onClick={onOpen}>
      <div className="entry-house-eyebrow">
        <MaxTypography.Label variant="medium-strong">ДОМ</MaxTypography.Label>
        {house.is_demo && <DemoBadge />}
      </div>
      <div className="entry-house-heading">
        <div className="entry-house-mark" aria-hidden="true">⌂</div>
        <div className="entry-house-address">
          <Typography.Title variant="medium-strong">{house.address}</Typography.Title>
          {house.region && <MaxTypography.Text variant="detail" color="secondary">{house.region}</MaxTypography.Text>}
        </div>
      </div>
      <div className="entry-house-divider" />
      <div className="between entry-house-summary">
        <MaxTypography.Text variant="detail" color="secondary">Квартиры в профиле</MaxTypography.Text>
        <MaxTypography.Label variant="medium-strong">{memberships.length}</MaxTypography.Label>
      </div>
      <div className="entry-apartments">
        {memberships.map((membership) => {
          const premise = membership.premise;
          const details = [
            premise.entrance != null ? `Подъезд ${premise.entrance}` : '',
            premise.floor != null ? `${premise.floor} этаж` : '',
            premise.display_area_m2 ? fmtM2Str(premise.display_area_m2) : '',
          ].filter(Boolean).join(' · ');
          return (
            <MaxCellAction type="button" mode="secondary" className="entry-apartment-card" key={membership.id} onClick={onOpen}>
              <div className="entry-apartment-title-row">
                <Apt n={premise.number} size={48} />
                <Typography.Title variant="medium-strong">Квартира {premise.number}</Typography.Title>
              </div>
              <div className="entry-apartment-copy">
                <div className="entry-apartment-details-row">
                  <MaxTypography.Text variant="detail" color="secondary">{details}</MaxTypography.Text>
                  <Status kind={membership.status === 'verified' ? 'ok' : membership.status === 'pending' && membership.role === 'owner' ? 'paper' : 'none'}>
                    {membership.status === 'verified' ? 'Подтверждена' : membership.status === 'pending' && membership.role === 'owner' ? 'На проверке' : membership.status === 'rejected' ? 'Отклонена' : 'Не подтверждена'}
                  </Status>
                </div>
                {membership.rejection_reason && <MaxTypography.Text variant="detail" color="secondary">Причина отказа: {membership.rejection_reason}</MaxTypography.Text>}
              </div>
            </MaxCellAction>
          );
        })}
      </div>
    </Card>
  );
}

export function Entry() {
  const navigate = useNavigate();
  const meState = useApi(() => fetchMe());
  const me = meState.data as MeResponse | null;
  const profile = getDisplayUser(me?.user);
  const houses = useMemo(() => {
    const grouped = new Map<string, MeResponse['memberships']>();
    for (const membership of me?.memberships ?? []) {
      const group = grouped.get(membership.house.id) ?? [];
      group.push(membership);
      grouped.set(membership.house.id, group);
    }
    return [...grouped.entries()].map(([houseId, memberships]) => ({ houseId, house: memberships[0].house, memberships }));
  }, [me?.memberships]);

  if (meState.loading) return <Screen><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка профиля…</MaxTypography.Text></Card></Main></Screen>;
  if (meState.error || !me) return <Screen><Main><Note kind="neg">Не удалось загрузить профиль. Откройте мини-приложение из MAX и попробуйте ещё раз.</Note></Main></Screen>;

  return (
    <Screen>
      <Main style={{ gap: 14, paddingTop: 16 }}>
        <Card className="card entry-profile-card" style={{ flexDirection: 'row', alignItems: 'center', gap: 14 }}>
          <UiAvatar src={profile.photoUrl || undefined} name={profile.firstName} />
          <div className="grow">
            <Typography.Title variant="large-strong">{[profile.firstName, profile.lastName].filter(Boolean).join(' ')}</Typography.Title>
            {profile.username && <Typography.Text variant="detail" color="secondary">@{profile.username.replace(/^@/, '')}</Typography.Text>}
          </div>
        </Card>

        <div className="between entry-apartments-heading">
          <Typography.Title variant="medium-strong">Мои квартиры</Typography.Title>
          <Typography.Text variant="detail" color="secondary">{me.memberships.length}</Typography.Text>
        </div>
        {houses.length ? houses.map(({ houseId, house, memberships }) => {
          const verified = memberships.find((m) => m.status === 'verified' && m.role === 'owner')
            ?? memberships.find((m) => m.status === 'verified')
            ?? memberships[0];
          const path = verified.role === 'owner' && verified.status === 'verified' ? '/home' : '/home/guest';
          return <HouseCard key={houseId} house={house} memberships={memberships} onOpen={() => navigate(`${path}?house=${encodeURIComponent(house.slug)}`)} />;
        }) : <Card className="card"><MaxTypography.Text className="t" variant="body">Вы ещё не добавили ни одной квартиры. Начните с кнопки ниже.</MaxTypography.Text></Card>}

      </Main>
      <Foot>
        <Btn onClick={() => navigate('/attach')}>Добавить квартиру</Btn>
      </Foot>
    </Screen>
  );
}

export function AttachHouse() {
  const navigate = useNavigate();
  const meState = useApi(() => fetchMe());
  const me = meState.data as MeResponse | null;
  const [slug, setSlug] = useState('');
  const [house, setHouse] = useState<HouseJSON | null>(null);
  const [selectedHouse, setSelectedHouse] = useState<HouseJSON | null>(null);
  const [premise, setPremise] = useState('');
  const [ownerIndex, setOwnerIndex] = useState('');
  const [results, setResults] = useState<HouseJSON[]>([]);
  const [attached, setAttached] = useState(false);
  const [membershipId, setMembershipId] = useState('');
  const [claimCandidates, setClaimCandidates] = useState<Owner[] | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState('');
  const searchSequence = useRef(0);

  const inviteHouse = me?.house ?? null;
  useEffect(() => {
    const query = slug.trim();
    const sequence = ++searchSequence.current;
    if (inviteHouse || house || selectedHouse || query.length < 3) {
      setResults([]);
      setSearching(false);
      return;
    }
    const timer = window.setTimeout(async () => {
      setSearching(true);
      setError('');
      try {
        const response = await searchHouses(query);
        if (sequence === searchSequence.current) setResults(response.houses);
      } catch (e) {
        if (sequence === searchSequence.current) {
          setResults([]);
          setError(e instanceof Error ? e.message : 'Не удалось найти дома');
        }
      } finally {
        if (sequence === searchSequence.current) setSearching(false);
      }
    }, 300);
    return () => window.clearTimeout(timer);
  }, [slug, inviteHouse, house, selectedHouse]);

  const chooseHouse = () => {
    const choice = inviteHouse ?? selectedHouse;
    if (!choice) return;
    setHouse(choice);
    setSelectedHouse(null);
    setResults([]);
    setError('');
  };

  const joinPremise = async () => {
    if (!house || !premise.trim() || submitting) return;
    setSubmitting(true);
    setError('');
    try {
      if (house.is_demo) {
        await confirmDemoMembership(house.slug, { premise_number: premise.trim(), ...(ownerIndex ? { owner_index: Number(ownerIndex) } : {}) });
        navigate('/', { replace: true });
      } else {
        const result = await attachGuest(house.slug, premise.trim());
        setMembershipId(result.membership_id);
        setAttached(true);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось отправить заявку');
    } finally {
      setSubmitting(false);
    }
  };

  const requestOwnerReview = async () => {
    if (!house || !premise.trim() || submitting) return;
    setSubmitting(true); setError('');
    try {
      const result = await attachGuest(house.slug, premise.trim());
      setMembershipId(result.membership_id);
      setAttached(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось создать заявку на проверку');
    } finally {
      setSubmitting(false);
    }
  };

  const verifyPhone = async () => {
    if (!membershipId || submitting) return;
    const webApp = window.WebApp;
    if (!webApp?.requestContact) { setError('Подтверждение телефона доступно при запуске мини-приложения в MAX.'); return; }
    setSubmitting(true); setError('');
    try {
      const contact = await webApp.requestContact();
      await verifyMembershipPhone(membershipId, { phone: contact.phone, auth_date: contact.authDate, hash: contact.hash });
      navigate('/', { replace: true });
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Номер не совпал с данными собственника. Можно попробовать ещё раз.');
    } finally { setSubmitting(false); }
  };

  const loadClaimCandidates = async () => {
    if (!membershipId || submitting) return;
    setSubmitting(true); setError('');
    try { setClaimCandidates((await fetchOwnerClaimCandidates(membershipId)).owners); }
    catch (e) { setError(e instanceof Error ? e.message : 'Не удалось загрузить список собственников'); }
    finally { setSubmitting(false); }
  };

  const claimOwner = async (owner: Owner) => {
    if (!membershipId || submitting) return;
    setSubmitting(true); setError('');
    try { await submitOwnerClaim(membershipId, owner.id); navigate('/', { replace: true }); }
    catch (e) { setError(e instanceof Error ? e.message : 'Не удалось отправить заявку в УК'); }
    finally { setSubmitting(false); }
  };

  const joinDemoStaff = async () => {
    if (!house || submitting) return;
    setSubmitting(true);
    setError('');
    try {
      await becomeDemoStaff(house.slug);
      navigate('/uk', { replace: true });
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Не удалось открыть кабинет УК');
    } finally {
      setSubmitting(false);
    }
  };

  if (meState.loading) return <Screen><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка…</MaxTypography.Text></Card></Main></Screen>;
  if (meState.error || !me) return <Screen><Main><Note kind="neg">Не удалось загрузить профиль.</Note></Main></Screen>;

  if (attached) return <Screen><div className="hdr"><MaxButton className="attach-back-button" type="button" variant="ghost" size="large" aria-label="Назад" onClick={() => navigate('/', { replace: true })}><span className="attach-back-glyph" aria-hidden="true">‹</span></MaxButton><div className="ttl"><Typography.Title variant="large-strong">Проверка собственника</Typography.Title></div></div><Main><Card className="card"><Typography.Title variant="medium-strong">{house?.address} · кв. {premise}</Typography.Title><Typography.Text variant="detail" color="secondary">Поделитесь подтверждённым номером MAX или отправьте заявку в УК</Typography.Text></Card><Note kind="info">Номер проверяется по реестру. Если он не совпадает, выберите свою запись собственника — УК подтвердит её вручную.</Note>{error && <Note kind="neg">{error}</Note>}<Btn onClick={verifyPhone} disabled={submitting}>{submitting ? 'Проверяем…' : 'Поделиться номером MAX'}</Btn>{!claimCandidates && <Btn kind="secondary" onClick={loadClaimCandidates} disabled={submitting}>Выбрать себя в реестре</Btn>}{claimCandidates && <UiList>{claimCandidates.map((owner) => <Cell key={owner.id} onClick={() => claimOwner(owner)} chevron><b>{owner.masked_name}</b><Typography.Text variant="detail" color="secondary">Доля {owner.share.numerator}/{owner.share.denominator} · голос {fmtM2Str(owner.weight_m2)}</Typography.Text></Cell>)}</UiList>}<Btn kind="text" onClick={() => navigate('/', { replace: true })}>Позже</Btn></Main></Screen>;

  return <Screen>
    <div className="hdr">
      <MaxButton className="attach-back-button" type="button" variant="ghost" size="large" aria-label="Назад" onClick={() => navigate(-1)}><span className="attach-back-glyph" aria-hidden="true">‹</span></MaxButton>
      <div className="ttl"><Typography.Title variant="large-strong">Прикрепление к дому</Typography.Title></div>
    </div>
    <Main style={{ gap: 14 }}>
      {!house && <>
        <MaxTypography.Text className="t" variant="body">Найдите дом по адресу или откройте QR-приглашение дома.</MaxTypography.Text>
        {inviteHouse && <Card className="card"><MaxTypography.Label className="lbl" variant="large-strong">Дом из ссылки MAX</MaxTypography.Label><MaxTypography.Headline className="h3" variant="small">{inviteHouse.address}</MaxTypography.Headline><MaxTypography.Text className="cap" variant="detail" color="secondary">{inviteHouse.slug}</MaxTypography.Text></Card>}
        {!inviteHouse && <>
          <label className="field attach-address-field"><Typography.Label variant="large-strong">Адрес дома</Typography.Label><UiInput value={slug} onChange={(e) => { setSlug(e.target.value); setSelectedHouse(null); setResults([]); setError(''); }} placeholder="Город, улица, дом" autoComplete="street-address" />
          </label>
          {slug.trim().length >= 3 && searching && <Typography.Text variant="detail" color="secondary">Ищем подходящие адреса…</Typography.Text>}
          {results.length > 0 && <div className="entry-search-results" role="listbox" aria-label="Найденные дома">
            {results.map((item) => <MaxCellAction type="button" mode="secondary" className={'entry-search-result' + (selectedHouse?.id === item.id ? ' selected' : '')} key={item.id} aria-selected={selectedHouse?.id === item.id} onClick={() => { setSelectedHouse(item); setSlug(item.address); setResults([]); setError(''); }}><div className="entry-search-result-copy"><Typography.Title variant="medium-strong">{item.address}</Typography.Title><Typography.Text variant="detail" color="secondary">{[item.region, item.locality].filter(Boolean).join(' · ')}</Typography.Text></div></MaxCellAction>)}
          </div>}
          {slug.trim().length >= 3 && !selectedHouse && !searching && !results.length && !error && <Typography.Text variant="detail" color="secondary">Совпадений не найдено. Проверьте адрес или уточните запрос.</Typography.Text>}
        </>}
        {error && <Note kind="neg">{error}</Note>}
      </>}
      {house && <>
        {house.is_demo && <DemoBadge />}
        <Card className="card"><MaxTypography.Headline className="h3" variant="small">{house.address}</MaxTypography.Headline><MaxTypography.Text className="cap" variant="detail" color="secondary">{house.region}</MaxTypography.Text></Card>
        <MaxTypography.Text className="t" variant="body">Укажите номер квартиры. Сначала профиль будет прикреплён к ней как гость.</MaxTypography.Text>
        <label className="field"><Typography.Label variant="large-strong">Номер квартиры</Typography.Label><UiInput value={premise} onChange={(e) => setPremise(e.target.value.trimStart())} /></label>
        {house.is_demo && <label className="field"><Typography.Label variant="large-strong">Индекс собственника, если их несколько</Typography.Label><UiInput inputMode="numeric" value={ownerIndex} onChange={(e) => setOwnerIndex(e.target.value.replace(/\D/g, ''))} placeholder="Обычно не нужен" />
        </label>}
        <Note kind="info">{house.is_demo ? 'Это учебный демо-дом: привязка и проверка собственника выполняются демонстрационно.' : 'После выбора квартиры сервер создаст гостевую привязку. Для автоматической проверки собственника нужен подтверждённый контакт MAX.'}</Note>
        {error && <Note kind="neg">{error}</Note>}
        <Btn onClick={joinPremise} disabled={submitting || !premise.trim()}>{submitting ? 'Прикрепляем…' : house.is_demo ? 'Подтвердить собственника в демо' : 'Продолжить'}</Btn>
        {house.is_demo && <Btn kind="secondary" onClick={requestOwnerReview} disabled={submitting || !premise.trim()}>{submitting ? 'Создаём заявку…' : 'Отправить заявку на проверку УК'}</Btn>}
        <Btn kind="text" onClick={() => { setHouse(null); setError(''); }}>Выбрать другой дом</Btn>
        {house.is_demo && <Btn kind="text" onClick={joinDemoStaff} disabled={submitting}>Я сотрудник УК · демо-вход</Btn>}
      </>}
    </Main>
    {!house && <Foot><Btn onClick={chooseHouse} disabled={!inviteHouse && !selectedHouse}>Это мой дом</Btn></Foot>}
  </Screen>;
}
