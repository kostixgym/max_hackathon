-- +goose Up
-- Модули «Реестр домов» и «Доступ и роли» (docs/04-model-dannyh.md).
-- Первичные ключи — UUIDv7 (uuidv7() есть в PostgreSQL 18).
-- Площади — целые сотые доли м² (52,30 м² = 5230), доли — числитель/знаменатель:
-- расчёты голосов точные, округление только при показе (решение 36).

CREATE TABLE organizations (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    type       text NOT NULL CHECK (type IN ('uk', 'tszh', 'zhsk')),
    name       text NOT NULL,
    inn        text,
    contacts   jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE houses (
    id                       uuid PRIMARY KEY DEFAULT uuidv7(),
    org_id                   uuid REFERENCES organizations (id),
    address                  text NOT NULL,
    fias_id                  text,
    region                   text NOT NULL,
    timezone                 text NOT NULL DEFAULT 'Europe/Moscow',
    passport_area_centi      bigint CHECK (passport_area_centi > 0),
    current_registry_version integer,
    -- Криптослучайный идентификатор для ссылки дома; публичный, прав не даёт (решение 57).
    invite_slug              text NOT NULL UNIQUE,
    is_demo                  boolean NOT NULL DEFAULT false,
    created_at               timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX houses_org_idx ON houses (org_id) WHERE org_id IS NOT NULL;
-- Демо-дом всегда один: ссылки на него в README и на слайде жюри не должны разъехаться.
CREATE UNIQUE INDEX houses_one_demo ON houses (is_demo) WHERE is_demo;

-- Помещение стабильно между версиями реестра (решение 53).
CREATE TABLE premises (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    house_id           uuid NOT NULL REFERENCES houses (id),
    number             text NOT NULL,
    kind               text NOT NULL CHECK (kind IN ('residential', 'nonresidential', 'parking')),
    entrance           integer,
    floor              integer,
    -- HMAC-SHA256 лицевого счёта с серверным секретом (решение 56).
    account_hmac       bytea,
    -- Кэш площади текущей версии только для отображения, в расчётах не используется (решение 71).
    display_area_centi bigint CHECK (display_area_centi > 0),
    UNIQUE (house_id, number)
);

-- Версия реестра. Общая площадь версии — база для порогов 10% / кворум / 2/3 (решение 55).
CREATE TABLE registry_uploads (
    id               uuid PRIMARY KEY DEFAULT uuidv7(),
    house_id         uuid NOT NULL REFERENCES houses (id),
    version          integer NOT NULL CHECK (version > 0),
    total_area_centi bigint NOT NULL CHECK (total_area_centi > 0),
    status           text NOT NULL CHECK (status IN ('preview', 'applied')),
    report           jsonb NOT NULL DEFAULT '{}'::jsonb,
    uploaded_by      uuid,
    uploaded_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (house_id, version)
);

-- Текущая версия дома должна существовать (составной ключ; при NULL проверка не выполняется).
-- Правило «текущей становится только применённая версия» проверяет код (registry).
ALTER TABLE houses
    ADD CONSTRAINT houses_current_registry_fk
    FOREIGN KEY (id, current_registry_version) REFERENCES registry_uploads (house_id, version);

-- Собственник стабилен между версиями реестра: это только ключ идентичности (решение 53).
CREATE TABLE owners (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    premise_id uuid NOT NULL REFERENCES premises (id),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX owners_premise_idx ON owners (premise_id);

-- Факты о собственнике в конкретной версии реестра.
-- Вес = площадь × доля в сотых м², точная дробь weight_num / weight_den.
CREATE TABLE owner_records (
    id                 uuid PRIMARY KEY DEFAULT uuidv7(),
    owner_id           uuid NOT NULL REFERENCES owners (id),
    registry_upload_id uuid NOT NULL REFERENCES registry_uploads (id),
    full_name          text NOT NULL,
    share_num          bigint NOT NULL CHECK (share_num > 0),
    share_den          bigint NOT NULL CHECK (share_den > 0),
    weight_num         bigint NOT NULL CHECK (weight_num > 0),
    weight_den         bigint NOT NULL CHECK (weight_den > 0),
    owner_kind         text NOT NULL CHECK (owner_kind IN ('person', 'organization', 'municipality')),
    phone_hmac         bytea,
    CHECK (share_num <= share_den),
    UNIQUE (owner_id, registry_upload_id)
);
CREATE INDEX owner_records_upload_idx ON owner_records (registry_upload_id);
CREATE INDEX owner_records_phone_idx ON owner_records (phone_hmac) WHERE phone_hmac IS NOT NULL;

CREATE TABLE users (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    max_user_id bigint NOT NULL UNIQUE,
    consent_at  timestamptz,
    deleted_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE registry_uploads
    ADD CONSTRAINT registry_uploads_uploaded_by_fk FOREIGN KEY (uploaded_by) REFERENCES users (id);

CREATE TABLE memberships (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id     uuid NOT NULL REFERENCES users (id),
    premise_id  uuid NOT NULL REFERENCES premises (id),
    owner_id    uuid REFERENCES owners (id),
    role        text NOT NULL CHECK (role IN ('guest', 'resident', 'owner')),
    method      text CHECK (method IN ('phone', 'account', 'uk_manual', 'demo')),
    status      text NOT NULL CHECK (status IN ('pending', 'verified', 'rejected', 'revoked')),
    verified_by uuid REFERENCES users (id),
    verified_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, premise_id),
    -- Привязка собственника всегда указывает на стабильного собственника.
    CHECK ((role = 'owner') = (owner_id IS NOT NULL))
);

CREATE INDEX memberships_premise_idx ON memberships (premise_id);
-- Поиск конфликтов «этого собственника уже заявил другой» — по всем статусам, не только подтверждённым.
CREATE INDEX memberships_owner_idx ON memberships (owner_id) WHERE owner_id IS NOT NULL;

-- Инвариант 9: за одним собственником не больше одной подтверждённой привязки.
CREATE UNIQUE INDEX memberships_one_verified_owner
    ON memberships (owner_id)
    WHERE role = 'owner' AND status = 'verified';

CREATE TABLE org_members (
    user_id    uuid NOT NULL REFERENCES users (id),
    org_id     uuid NOT NULL REFERENCES organizations (id),
    role       text NOT NULL CHECK (role IN ('admin', 'operator')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, org_id)
);

-- +goose Down
DROP TABLE org_members;
DROP TABLE memberships;
ALTER TABLE registry_uploads DROP CONSTRAINT registry_uploads_uploaded_by_fk;
ALTER TABLE houses DROP CONSTRAINT houses_current_registry_fk;
DROP TABLE users;
DROP TABLE owner_records;
DROP TABLE owners;
DROP TABLE registry_uploads;
DROP TABLE premises;
DROP TABLE houses;
DROP TABLE organizations;
