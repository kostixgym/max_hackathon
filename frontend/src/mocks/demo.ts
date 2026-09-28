// Демо-данные экранов. Формы объектов повторяют контракт docs/API_DESCRIPTION.md,
// площади — числами для удобства расчётов на моках (в API это строки "3000.00").

export const house = {
  id: 'demo-house-id',
  slug: 'demo-house',
  address: 'г. Казань, ул. Демонстрационная, д. 1',
  shortAddress: 'Казань, ул. Демонстрационная, д. 1',
  isDemo: true,
  flats: 60,
  totalAreaM2: 3000,
  thresholds: { demandM2: 300, quorumAboveM2: 1500, twoThirdsM2: 2000 },
  /** средняя площадь квартиры для оценки «~N кв.» */
  avgAreaM2: 50,
  uk: 'УК «Демо-Сервис»',
};

export const me = {
  name: 'Анна',
  premise: { number: '45', entrance: 2, floor: 5, areaM2: 52.3 },
  share: { numerator: 1, denominator: 2 },
  weightM2: 26.15,
};

export const initiative = {
  id: 'cameras',
  title: 'Камеры в подъездах',
  initiator: 'Анна, кв. 45',
  createdAt: '8 сентября',
  pollEndsAt: '5 октября',
  cameras: 6,
  payment: '+150 ₽/мес отдельной строкой',
  records: 'Хранит УК',
  description:
    'Поставить 6 камер: у входов, в лифтовых холлах и на первых этажах обоих подъездов. Цель — меньше порчи лифтов и краж колясок.',
  poll: { forM2: 1240, votedM2: 1420, answered: 27 },
};

export const agenda = [
  { short: '1. Председатель и секретарь', full: '1. Выбрать председателя и секретаря собрания', rule: '>50% участников' },
  { short: '2. Установка 6 камер', full: '2. Установить 6 камер в подъездах', rule: '2/3 дома' },
  { short: '3. Оплата 150 ₽/мес', full: '3. Оплата 150 ₽ в месяц отдельной строкой в квитанции', rule: '2/3 дома' },
  { short: '4. Хранение записей', full: '4. Записи хранит УК и выдаёт по запросу', rule: '>50% участников' },
];

const areas = [64.1, 38.4, 52.3];

/** Квартиры демо-дома: 2 подъезда по 30 квартир, 3 квартиры на этаже. */
export function flat(n: number) {
  const k = (n - 1) % 30;
  return { n, entrance: n <= 30 ? 1 : 2, floor: Math.floor(k / 3) + 1, areaM2: areas[(n - 1) % 3] };
}

export const flats = Array.from({ length: house.flats }, (_, i) => flat(i + 1));

export type TrackerStatus = 'ok' | 'paper' | 'said' | 'none' | 'bad';

// Статусы трекера собрания (как в макете «13. Трекер квартир»).
const trackerPattern = 'oopnsoonpoesonnoposonoopnsoonpoonsnpooosnonpoosonnopsoonpson';
const code: Record<string, TrackerStatus> = { o: 'ok', p: 'paper', s: 'said', n: 'none', e: 'bad' };
export const tracker: TrackerStatus[] = flats.map((_, i) => code[trackerPattern.charAt(i)] ?? 'none');
