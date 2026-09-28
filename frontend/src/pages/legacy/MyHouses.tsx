import { Icon } from '../../components/Icon';
import { IconButton, UiButton } from '../../components/ui';

// «v1 · Мои дома» — отдельная ранняя страница из макета (дома и проблемы в них).
// Нарисована в своём стиле, без токенов maxkit; переносим как есть.

type ProblemStatus = 'open' | 'work' | 'done';
const statusView: Record<ProblemStatus, { label: string; bg: string; color: string }> = {
  open: { label: 'Открыта', bg: '#ffe8ea', color: '#c4121d' },
  work: { label: 'В работе', bg: '#fff1e0', color: '#a34c00' },
  done: { label: 'Решена', bg: '#e3f7e7', color: '#177a2c' },
};

type House = {
  address: string;
  city: string;
  desc: string;
  tags: string[];
  problems: [string, string, ProblemStatus][];
};

const houses: House[] = [
  {
    address: 'ул. Ленина, 12',
    city: 'Санкт-Петербург, Петроградский р-н',
    desc: 'Кирпичная пятиэтажка, 4 подъезда. УК «Петроградец», совет дома собирается раз в квартал.',
    tags: ['1968 г.', '5 этажей', '60 кв.', 'Кирпич'],
    problems: [
      ['Протечка крыши над 4 подъездом', '12.09', 'work'],
      ['Не работает домофон, подъезд 2', '20.09', 'open'],
      ['Перегорели лампы на лестнице', '02.09', 'done'],
    ],
  },
  {
    address: 'пр. Просвещения, 87 к. 1',
    city: 'Санкт-Петербург, Выборгский р-н',
    desc: 'Панельный дом, 6 подъездов, лифты в каждом. Своя котельная на территории.',
    tags: ['1985 г.', '12 этажей', '144 кв.', 'Панель'],
    problems: [
      ['Лифт в 1 подъезде застревает между этажами', '23.09', 'open'],
      ['Трещина в фасаде у 5 подъезда', '15.08', 'work'],
      ['Засор мусоропровода', '10.09', 'done'],
      ['Нет горячей воды в стояке 3', '24.09', 'open'],
    ],
  },
  {
    address: 'ул. Гороховая, 45',
    city: 'Санкт-Петербург, Адмиралтейский р-н',
    desc: 'Исторический доходный дом, объект культурного наследия. Двор-колодец, парадная с лепниной.',
    tags: ['1902 г.', '4 этажа', '24 кв.', 'Кирпич'],
    problems: [
      ['Сырость и плесень в подвале', '05.09', 'open'],
      ['Осыпается лепнина в парадной', '28.07', 'work'],
    ],
  },
  {
    address: 'ул. Звёздная, 8',
    city: 'Санкт-Петербург, Московский р-н',
    desc: 'Монолитный ЖК с подземным паркингом и закрытым двором. Консьерж в каждом подъезде.',
    tags: ['2014 г.', '17 этажей', '280 кв.', 'Монолит'],
    problems: [
      ['Гул вентиляции паркинга ночью', '18.09', 'work'],
      ['Сломан шлагбаум у въезда', '11.09', 'done'],
      ['Не закрывается калитка во двор', '22.09', 'open'],
    ],
  },
  {
    address: 'ул. Садовая, 3',
    city: 'Колпино',
    desc: 'Девятиэтажка у парка, 3 подъезда. В 2023 году прошёл капремонт кровли и фасада.',
    tags: ['1978 г.', '9 этажей', '108 кв.', 'Панель'],
    problems: [
      ['Покраска перил в подъездах', '01.09', 'work'],
      ['Замена почтовых ящиков', '14.08', 'done'],
    ],
  },
];

const C = { bg: '#f5f7fa', text: '#1d1e20', text2: '#6b6e73', acc: '#007aff', accSoft: '#e6f1ff', chip: '#f0f2f5', body: '#2b2c2e' };

const card: React.CSSProperties = { background: '#fff', borderRadius: 16, padding: 16, display: 'flex', flexDirection: 'column' };
const grid: React.CSSProperties = { display: 'grid', gridTemplateColumns: 'minmax(0, 1fr) 48px 84px', gap: 8 };

function Profile({ counts }: { counts: [number, number, number] }) {
  const stats: [number, string, string?][] = [
    [counts[0], 'домов'],
    [counts[1], 'открыто', counts[1] ? '#d6202b' : undefined],
    [counts[2], 'в работе', counts[2] ? '#b35400' : undefined],
  ];
  return (
    <section style={{ ...card, gap: 16 }} aria-label="Профиль">
      <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
        <div className="av" style={{ width: 64, height: 64, fontSize: 24 }}>
          ИП
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 2, flexGrow: 1, minWidth: 0 }}>
          <div style={{ fontSize: 20, fontWeight: 600, lineHeight: '26px' }}>Иван Петров</div>
          <div style={{ fontSize: 15, color: C.text2, lineHeight: '20px' }}>@ivan_petrov</div>
          <div style={{ fontSize: 13, color: C.text2, lineHeight: '18px' }}>ID 104 582 331 · ru</div>
        </div>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 8 }}>
        {stats.map(([n, label, color]) => (
          <div key={label} style={{ background: C.bg, borderRadius: 12, padding: '10px 12px', display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div style={{ fontSize: 20, fontWeight: 600, color }}>{n}</div>
            <div style={{ fontSize: 12, color: C.text2 }}>{label}</div>
          </div>
        ))}
      </div>
    </section>
  );
}

function AddHouse() {
  return (
    <UiButton type="button" variant="primary" size="large" style={{ marginTop: 4 }}>
      <Icon name="plus" small />
      Добавить дом
    </UiButton>
  );
}

function Page({ list }: { list: House[] }) {
  const all = list.flatMap((h) => h.problems);
  const count = (s: ProblemStatus) => all.filter((p) => p[2] === s).length;
  return (
    <div
      className="kit"
      style={{
        maxWidth: 480,
        margin: '0 auto',
        minHeight: '100dvh',
        boxSizing: 'border-box',
        background: C.bg,
        color: C.text,
        padding: '12px 16px 32px',
        display: 'flex',
        flexDirection: 'column',
        gap: 12,
        fontFamily: '-apple-system, system-ui, "Helvetica Neue", Roboto, sans-serif',
      }}
    >
      <Profile counts={[list.length, count('open'), count('work')]} />

      <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', padding: '8px 4px 0' }}>
        <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>Мои дома</h1>
        <span style={{ fontSize: 13, color: C.text2 }}>{list.length ? `${list.length} объектов` : 'нет объектов'}</span>
      </div>

      {list.length === 0 && (
        <section style={{ ...card, padding: '40px 24px', alignItems: 'center', gap: 12, textAlign: 'center' }}>
          <div style={{ width: 72, height: 72, borderRadius: 999, background: C.accSoft, color: C.acc, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Icon name="house" style={{ width: 34, height: 34, strokeWidth: 1.8 }} />
          </div>
          <div style={{ fontSize: 17, fontWeight: 600, lineHeight: '22px' }}>Пока нет ни одного дома</div>
          <p style={{ margin: 0, fontSize: 15, lineHeight: '20px', color: C.text2, maxWidth: 280 }}>Добавьте свой дом, чтобы отмечать проблемы и следить за их решением</p>
        </section>
      )}

      {list.map((h) => (
        <article key={h.address} style={{ ...card, gap: 12 }}>
          <div style={{ display: 'flex', gap: 12, alignItems: 'flex-start' }}>
            <div style={{ width: 44, height: 44, borderRadius: 12, background: C.accSoft, color: C.acc, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
              <Icon name="house" style={{ width: 22, height: 22 }} />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 2, flexGrow: 1, minWidth: 0 }}>
              <div style={{ fontSize: 17, fontWeight: 600, lineHeight: '22px' }}>{h.address}</div>
              <div style={{ fontSize: 13, color: C.text2, lineHeight: '18px' }}>{h.city}</div>
            </div>
            <IconButton icon="more" label="Действия с домом" />
          </div>
          <p style={{ margin: 0, fontSize: 15, lineHeight: '20px', color: C.body }}>{h.desc}</p>
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
            {h.tags.map((t) => (
              <span key={t} style={{ fontSize: 13, padding: '4px 10px', borderRadius: 999, background: C.chip, color: C.body }}>
                {t}
              </span>
            ))}
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', borderTop: '1px solid rgba(12,13,14,.08)', paddingTop: 8 }}>
            <div style={{ ...grid, padding: '4px 0 8px', fontSize: 12, fontWeight: 500, letterSpacing: 0.5, textTransform: 'uppercase', color: C.text2 }}>
              <span>Проблема</span>
              <span>Дата</span>
              <span style={{ textAlign: 'right' }}>Статус</span>
            </div>
            {h.problems.map(([title, date, s]) => (
              <div key={title} style={{ ...grid, alignItems: 'center', padding: '10px 0', borderTop: '1px solid rgba(12,13,14,.06)' }}>
                <span style={{ fontSize: 14, lineHeight: '19px' }}>{title}</span>
                <span style={{ fontSize: 13, color: C.text2 }}>{date}</span>
                <span style={{ display: 'flex', justifyContent: 'flex-end' }}>
                  <span style={{ display: 'inline-block', fontSize: 12, fontWeight: 500, padding: '4px 8px', borderRadius: 999, whiteSpace: 'nowrap', background: statusView[s].bg, color: statusView[s].color }}>
                    {statusView[s].label}
                  </span>
                </span>
              </div>
            ))}
          </div>
        </article>
      ))}

      <AddHouse />
    </div>
  );
}

// Главная — 5 домов
export function MyHouses() {
  return <Page list={houses} />;
}

// Главная — 0 домов
export function MyHousesEmpty() {
  return <Page list={[]} />;
}
