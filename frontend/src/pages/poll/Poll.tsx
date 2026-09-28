import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { ProgressM2 } from '../../components/ProgressM2';
import { Apt, Badge, Btn, Foot, Header, Kv, LinkBtn, Main, Opt, PollDisclaimer, Screen, Seg, StateHead, Status, UiList, Card } from '../../components/ui';
import { fmtM2, fmtNum } from '../../lib/format';
import { flat, initiative, me } from '../../mocks/demo';
import { P } from '../../paths';

// 8. Голос в опросе
export function Vote() {
  const [choice, setChoice] = useState<'for' | 'against'>('for');
  return (
    <Screen>
      <Header title="Опрос" />
      <Main style={{ gap: 14 }}>
        <MaxTypography.Headline className="h2" style={{ padding: '0 4px' }} variant="medium">
          Вы за установку камер в подъездах?
        </MaxTypography.Headline>
        <Card className="card" style={{ gap: 8 }}>
          <Kv k="Камер">{String(initiative.cameras)}</Kv>
          <Kv k="Оплата">+150 ₽/мес в квитанции</Kv>
          <Kv k="Записи">{initiative.records}</Kv>
        </Card>
        <Card className="card" style={{ flexDirection: 'row', alignItems: 'center', gap: 12 }}>
          <div className="grow">
            <MaxTypography.Text className="cap" variant="detail" color="secondary">Ваш голос</MaxTypography.Text>
            <MaxTypography.Display className="big" style={{ fontSize: 28, lineHeight: '34px' }}>
              {fmtM2(me.weightM2)}
            </MaxTypography.Display>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">
              {fmtNum(me.premise.areaM2)} м² × доля {me.share.numerator}/{me.share.denominator}
            </MaxTypography.Text>
          </div>
          <Badge kind="calc" style={{ alignSelf: 'center' }} />
        </Card>
        <div className="col" style={{ gap: 10 }} role="radiogroup" aria-label="Ваш ответ">
          <Opt big on={choice === 'for'} onClick={() => setChoice('for')} title="За" />
          <Opt big on={choice === 'against'} onClick={() => setChoice('against')} title="Против" />
        </div>
        <PollDisclaimer />
      </Main>
      <Foot>
        {/* Анкету спрашиваем только у голосующих «за» (решение 62) */}
        <Btn to={choice === 'for' ? P.survey : P.counted}>Отправить ответ</Btn>
      </Foot>
    </Screen>
  );
}

// 8б. Анкета после «За»
export function Survey() {
  const [channel, setChannel] = useState('gosuslugi');
  const [help, setHelp] = useState<string[]>(['chat']);
  const toggleHelp = (v: string) =>
    setHelp((h) => (v === 'no' ? (h.includes('no') ? [] : ['no']) : h.includes(v) ? h.filter((x) => x !== v) : [...h.filter((x) => x !== 'no'), v]));

  return (
    <Screen>
      <Header title="Ещё два вопроса" right={<MaxTypography.Text className="cap" variant="detail" color="secondary">2 из 2</MaxTypography.Text>} />
      <Main style={{ gap: 14 }}>
        <div className="row" style={{ padding: '0 4px' }}>
          <Status kind="ok">Вы — «за»</Status>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">ответ сохранён</MaxTypography.Text>
        </div>
        <Card className="card" style={{ gap: 10 }}>
          <MaxTypography.Headline className="h3" variant="small">Как вам удобнее голосовать официально?</MaxTypography.Headline>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">Настоящее голосование будет позже — на собрании.</MaxTypography.Text>
          <Opt on={channel === 'gosuslugi'} onClick={() => setChannel('gosuslugi')} title="В Госуслуги.Дом" caption="Онлайн, с телефона" />
          <Opt on={channel === 'paper'} onClick={() => setChannel('paper')} title="Бумажный бюллетень" caption="Принесут домой, заберут заполненный" />
          <Opt on={channel === 'unknown'} onClick={() => setChannel('unknown')} title="Пока не знаю" />
        </Card>
        <Card className="card" style={{ gap: 10 }}>
          <MaxTypography.Headline className="h3" variant="small">Готовы помочь собрать голоса?</MaxTypography.Headline>
          <Opt check on={help.includes('floor')} onClick={() => toggleHelp('floor')} title="Обойду соседей на своём этаже" />
          <Opt check on={help.includes('chat')} onClick={() => toggleHelp('chat')} title="Напишу в чат подъезда" />
          <Opt check on={help.includes('no')} onClick={() => toggleHelp('no')} title="Пока не готов(а)" />
        </Card>
      </Main>
      <Foot>
        <Btn to={P.counted}>Готово</Btn>
        <Btn kind="text" to={P.counted}>
          Пропустить
        </Btn>
      </Foot>
    </Screen>
  );
}

// 8в. Голос учтён
export function Counted() {
  const { forM2, votedM2 } = initiative.poll;
  return (
    <Screen>
      <Header nav="close" title="" />
      <Main style={{ gap: 16, paddingTop: 16 }}>
        <StateHead icon="check" tone="pos" title="Голос учтён" text={`Вы — «за». Ваш голос в опросе: ${fmtM2(me.weightM2)}.`} />
        <Card className="card" style={{ gap: 12 }}>
          <ProgressM2 yes={forM2} voted={votedM2} compact title="Опрос по камерам" badge="poll" cap={`С вашим голосом: ${fmtM2(forM2)} «за» из ${fmtM2(2000)} нужных`} />
        </Card>
        <PollDisclaimer>Это опрос, а не голосование собрания. Официально голосовать нужно будет позже — мы напомним.</PollDisclaimer>
        <LinkBtn icon="edit" style={{ alignSelf: 'center' }} to={P.vote}>
          Изменить ответ
        </LinkBtn>
      </Main>
      <Foot>
        <Btn to={P.initPoll}>Вернуться к инициативе</Btn>
      </Foot>
    </Screen>
  );
}

const answered = [1, 2, 4, 7, 8, 10, 11];

// 9. Ход опроса — инициатор
export function PollProgress() {
  const [filter, setFilter] = useState<'all' | 'yes' | 'no'>('all');
  const list = Array.from({ length: 11 }, (_, i) => flat(i + 1))
    .map((f) => ({ ...f, ok: answered.includes(f.n) }))
    .filter((f) => filter === 'all' || (filter === 'yes' ? f.ok : !f.ok));

  return (
    <Screen>
      <Header title="Ход опроса" right={<MaxTypography.Text className="cap" variant="detail" color="secondary">до 5 окт</MaxTypography.Text>} />
      <Main style={{ gap: 14 }}>
        <Card className="card">
          <ProgressM2 yes={initiative.poll.forM2} voted={initiative.poll.votedM2} title="Ответы соседей" badge="poll" />
        </Card>

        <Card className="card" style={{ gap: 10, boxShadow: 'inset 0 0 0 1px var(--mod)' }}>
          <div className="between">
            <MaxTypography.Headline className="h3" variant="small">Хватит ли на 2/3?</MaxTypography.Headline>
            <Badge kind="model" />
          </div>
          <MaxTypography.Text className="t" variant="body">
            Скорее да. Если остальные ответят так же, как уже ответившие (87% «за»), «за» будет около <b>2 620 м²</b> — это больше 2 000 м².
          </MaxTypography.Text>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">Прогноз, а не факт: те, кто не ответил, часто менее заинтересованы.</MaxTypography.Text>
        </Card>

        <div className="between" style={{ padding: '6px 4px 0' }}>
          <MaxTypography.Headline className="h3" variant="small">Квартиры</MaxTypography.Headline>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">ответили {initiative.poll.answered} из 60</MaxTypography.Text>
        </div>
        <Seg
          value={filter}
          onChange={setFilter}
          options={[
            { value: 'all', label: 'Все' },
            { value: 'yes', label: 'Ответили' },
            { value: 'no', label: 'Нет ответа' },
          ]}
        />
        <MaxTypography.Text className="cap" style={{ padding: '0 4px' }} variant="detail" color="secondary">
          Показываем только, ответил ли сосед. Как он проголосовал — не видно никому.
        </MaxTypography.Text>
        <UiList>
          {list.map((f) => (
            <div key={f.n} className="cell" style={{ minHeight: 60 }}>
              <Apt n={f.n} size={40} />
              <span className="grow">
                <span style={{ fontWeight: 600 }}>Кв. {f.n}</span>
                <MaxTypography.Text className="cap" variant="detail" color="secondary">
                  Подъезд {f.entrance} · этаж {f.floor}
                </MaxTypography.Text>
              </span>
              <Status kind={f.ok ? 'ok' : 'none'}>{f.ok ? 'Ответил' : 'Нет ответа'}</Status>
            </div>
          ))}
        </UiList>
      </Main>
      <Foot>
        <Btn icon="share">Напомнить соседям в чате дома</Btn>
      </Foot>
    </Screen>
  );
}
