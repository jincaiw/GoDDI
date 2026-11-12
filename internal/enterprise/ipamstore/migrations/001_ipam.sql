-- GoDDI / SQLite: IPAM 约束参考迁移。对空库执行一次；不包含账号、DNS、DHCP 租约、
-- 审计、幂等和复制日志。运行时每个连接均须启用 foreign_keys。
PRAGMA foreign_keys = ON;
BEGIN IMMEDIATE;

-- 持久身份账本：释放不允许复用旧 ID；随业务库备份、快照和重放。
-- 空串只作为没有上级范围的账本键，不是业务 ID。
CREATE TABLE ipam_resource_ids (
    kind         TEXT NOT NULL CHECK (kind IN ('TENANT','SPACE','PREFIX','POOL','OCCUPANCY')),
    tenant_id    TEXT NOT NULL,
    space_id     TEXT NOT NULL,
    id           TEXT NOT NULL CHECK (id <> ''),
    PRIMARY KEY (kind, tenant_id, space_id, id),
    CHECK ((kind = 'TENANT' AND tenant_id = '' AND space_id = '')
        OR (kind = 'SPACE' AND tenant_id <> '' AND space_id = '')
        OR (kind IN ('PREFIX','POOL','OCCUPANCY') AND tenant_id <> '' AND space_id <> ''))
) STRICT;

CREATE TRIGGER ipam_resource_ids_no_replace
BEFORE INSERT ON ipam_resource_ids
WHEN EXISTS (SELECT 1 FROM ipam_resource_ids r
             WHERE r.kind = NEW.kind AND r.tenant_id = NEW.tenant_id AND r.space_id = NEW.space_id AND r.id = NEW.id)
BEGIN
    SELECT RAISE(ABORT, 'IPAM_ID_ALREADY_USED');
END;

CREATE TRIGGER ipam_resource_ids_no_update
BEFORE UPDATE ON ipam_resource_ids
BEGIN
    SELECT RAISE(ABORT, 'IPAM_ID_LEDGER_IMMUTABLE');
END;

CREATE TRIGGER ipam_resource_ids_no_delete
BEFORE DELETE ON ipam_resource_ids
BEGIN
    SELECT RAISE(ABORT, 'IPAM_ID_LEDGER_IMMUTABLE');
END;

CREATE TABLE ipam_tenants (
    id           TEXT NOT NULL PRIMARY KEY CHECK (id <> ''),
    slug         TEXT NOT NULL UNIQUE CHECK (slug <> ''),
    display_name TEXT NOT NULL CHECK (display_name <> ''),
    revision     INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0)
) STRICT;

CREATE TABLE ipam_spaces (
    tenant_id    TEXT NOT NULL,
    id           TEXT NOT NULL CHECK (id <> ''),
    slug         TEXT NOT NULL CHECK (slug <> ''),
    display_name TEXT NOT NULL CHECK (display_name <> ''),
    revision     INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, slug),
    FOREIGN KEY (tenant_id) REFERENCES ipam_tenants(id) ON DELETE RESTRICT
) STRICT;

CREATE TABLE ipam_prefixes (
    tenant_id  TEXT NOT NULL,
    space_id   TEXT NOT NULL,
    id         TEXT NOT NULL CHECK (id <> ''),
    parent_id  TEXT,
    family     INTEGER NOT NULL CHECK (family IN (4, 6)),
    prefix_len INTEGER NOT NULL CHECK (prefix_len BETWEEN 0 AND (CASE family WHEN 4 THEN 32 ELSE 128 END)),
    first_ip   BLOB NOT NULL CHECK (length(first_ip) = 16),
    last_ip    BLOB NOT NULL CHECK (length(last_ip) = 16),
    kind       TEXT NOT NULL CHECK (kind IN ('CONTAINER', 'SUBNET')),
    usage      TEXT NOT NULL CHECK (usage IN ('AGGREGATE', 'STANDARD', 'P2P', 'HOST')),
    site_ref   TEXT,
    label      TEXT NOT NULL DEFAULT '',
    revision   INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    -- IPv4 的高 96 位为零，因此 16 字节 BLOB 可统一校验 CIDR 对齐。
    binary_prefix_bits INTEGER GENERATED ALWAYS AS (prefix_len + CASE family WHEN 4 THEN 96 ELSE 0 END) VIRTUAL,
    PRIMARY KEY (tenant_id, space_id, id),
    FOREIGN KEY (tenant_id, space_id) REFERENCES ipam_spaces(tenant_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (tenant_id, space_id, parent_id)
        REFERENCES ipam_prefixes(tenant_id, space_id, id) ON DELETE RESTRICT,
    CHECK (parent_id IS NULL OR parent_id <> id),
    CHECK (first_ip <= last_ip),
    CHECK (family <> 4 OR (substr(first_ip, 1, 12) = zeroblob(12) AND substr(last_ip, 1, 12) = zeroblob(12))),
    CHECK (family <> 6 OR prefix_len < 96 OR substr(first_ip, 1, 12) <> x'00000000000000000000FFFF'),
    CHECK (substr(hex(first_ip), 1, binary_prefix_bits / 4) = substr(hex(last_ip), 1, binary_prefix_bits / 4)),
    CHECK (rtrim(substr(hex(first_ip), (binary_prefix_bits + 3) / 4 + 1), '0') = ''),
    CHECK (rtrim(substr(hex(last_ip), (binary_prefix_bits + 3) / 4 + 1), 'F') = ''),
    CHECK (binary_prefix_bits % 4 = 0 OR (
        (instr('0123456789ABCDEF', substr(hex(first_ip), binary_prefix_bits / 4 + 1, 1)) - 1)
            % (1 << (4 - binary_prefix_bits % 4)) = 0
        AND instr('0123456789ABCDEF', substr(hex(last_ip), binary_prefix_bits / 4 + 1, 1))
            - instr('0123456789ABCDEF', substr(hex(first_ip), binary_prefix_bits / 4 + 1, 1))
            = (1 << (4 - binary_prefix_bits % 4)) - 1
    )),
    CHECK ((kind = 'CONTAINER' AND usage = 'AGGREGATE')
        OR (kind = 'SUBNET' AND (
            (usage = 'STANDARD' AND prefix_len < CASE family WHEN 4 THEN 31 ELSE 127 END)
            OR (usage = 'P2P' AND prefix_len = CASE family WHEN 4 THEN 31 ELSE 127 END)
            OR (usage = 'HOST' AND prefix_len = CASE family WHEN 4 THEN 32 ELSE 128 END)
        )))
) STRICT;

CREATE INDEX ipam_prefixes_sibling_idx
    ON ipam_prefixes(tenant_id, space_id, parent_id, family, first_ip, last_ip);

CREATE TRIGGER ipam_prefixes_insert_guard
BEFORE INSERT ON ipam_prefixes
BEGIN
    SELECT RAISE(ABORT, 'IPAM_ID_ALREADY_USED')
    WHERE EXISTS (SELECT 1 FROM ipam_resource_ids r
                  WHERE r.kind = 'PREFIX' AND r.tenant_id = NEW.tenant_id AND r.space_id = NEW.space_id AND r.id = NEW.id);
    SELECT RAISE(ABORT, 'IPAM_PARENT_INVALID')
    WHERE NEW.parent_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM ipam_prefixes p
        WHERE p.tenant_id = NEW.tenant_id AND p.space_id = NEW.space_id AND p.id = NEW.parent_id
          AND p.kind = 'CONTAINER' AND p.family = NEW.family AND p.prefix_len < NEW.prefix_len
          AND p.first_ip <= NEW.first_ip AND p.last_ip >= NEW.last_ip
    );
    SELECT RAISE(ABORT, 'IPAM_PREFIX_OVERLAP')
    WHERE EXISTS (
        SELECT 1 FROM ipam_prefixes p
        WHERE p.tenant_id = NEW.tenant_id AND p.space_id = NEW.space_id AND p.parent_id IS NEW.parent_id
          AND p.family = NEW.family AND p.first_ip <= NEW.last_ip AND p.last_ip >= NEW.first_ip
    );
END;

CREATE TRIGGER ipam_prefixes_record_id
AFTER INSERT ON ipam_prefixes
BEGIN
    INSERT INTO ipam_resource_ids VALUES ('PREFIX', NEW.tenant_id, NEW.space_id, NEW.id);
END;

-- 调整拓扑须显式变更计划、迁移依赖后删除重建；普通 PATCH 只能更新元数据。
CREATE TRIGGER ipam_prefixes_topology_immutable
BEFORE UPDATE OF tenant_id, space_id, id, parent_id, family, prefix_len, first_ip, last_ip, kind, usage
ON ipam_prefixes
WHEN NEW.tenant_id IS NOT OLD.tenant_id OR NEW.space_id IS NOT OLD.space_id OR NEW.id IS NOT OLD.id
  OR NEW.parent_id IS NOT OLD.parent_id OR NEW.family IS NOT OLD.family OR NEW.prefix_len IS NOT OLD.prefix_len
  OR NEW.first_ip IS NOT OLD.first_ip OR NEW.last_ip IS NOT OLD.last_ip
  OR NEW.kind IS NOT OLD.kind OR NEW.usage IS NOT OLD.usage
BEGIN
    SELECT RAISE(ABORT, 'IPAM_PREFIX_TOPOLOGY_IMMUTABLE');
END;

CREATE TABLE ipam_pools (
    tenant_id TEXT NOT NULL,
    space_id  TEXT NOT NULL,
    id        TEXT NOT NULL CHECK (id <> ''),
    subnet_id TEXT NOT NULL,
    family    INTEGER NOT NULL CHECK (family IN (4, 6)),
    pool_kind TEXT NOT NULL CHECK (pool_kind IN ('IPAM', 'DHCP')),
    first_ip  BLOB NOT NULL CHECK (length(first_ip) = 16),
    last_ip   BLOB NOT NULL CHECK (length(last_ip) = 16),
    label     TEXT NOT NULL DEFAULT '',
    revision  INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    PRIMARY KEY (tenant_id, space_id, id),
    FOREIGN KEY (tenant_id, space_id, subnet_id)
        REFERENCES ipam_prefixes(tenant_id, space_id, id) ON DELETE RESTRICT,
    CHECK (first_ip <= last_ip),
    CHECK (family <> 4 OR (substr(first_ip, 1, 12) = zeroblob(12) AND substr(last_ip, 1, 12) = zeroblob(12))),
    -- 不允许 IPv6 动态区间穿过 IPv4-mapped 地址空间。
    CHECK (family <> 6 OR last_ip < x'00000000000000000000FFFF00000000'
        OR first_ip > x'00000000000000000000FFFFFFFFFFFF')
) STRICT;

CREATE INDEX ipam_pools_subnet_range_idx
    ON ipam_pools(tenant_id, space_id, subnet_id, first_ip, last_ip);

CREATE TABLE ipam_occupancies (
    tenant_id    TEXT NOT NULL,
    space_id     TEXT NOT NULL,
    id           TEXT NOT NULL CHECK (id <> ''),
    subnet_id    TEXT NOT NULL,
    family       INTEGER NOT NULL CHECK (family IN (4, 6)),
    address      BLOB NOT NULL CHECK (length(address) = 16),
    kind         TEXT NOT NULL CHECK (kind IN ('HOLD', 'STATIC', 'RESERVATION', 'EXCLUDED')),
    owner_ref    TEXT NOT NULL CHECK (owner_ref <> ''),
    expires_at_ms INTEGER,
    created_at_ms INTEGER NOT NULL CHECK (created_at_ms >= 0),
    revision     INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    PRIMARY KEY (tenant_id, space_id, id),
    UNIQUE (tenant_id, space_id, family, address),
    FOREIGN KEY (tenant_id, space_id, subnet_id)
        REFERENCES ipam_prefixes(tenant_id, space_id, id) ON DELETE RESTRICT,
    CHECK ((kind = 'HOLD' AND expires_at_ms IS NOT NULL AND expires_at_ms > created_at_ms)
        OR (kind <> 'HOLD' AND expires_at_ms IS NULL)),
    CHECK (family <> 4 OR substr(address, 1, 12) = zeroblob(12)),
    CHECK (family <> 6 OR substr(address, 1, 12) <> x'00000000000000000000FFFF')
) STRICT;

CREATE INDEX ipam_occupancies_subnet_addr_idx
    ON ipam_occupancies(tenant_id, space_id, subnet_id, address);

CREATE INDEX ipam_occupancies_hold_expiry_idx
    ON ipam_occupancies(tenant_id, expires_at_ms, id) WHERE kind = 'HOLD';

CREATE TRIGGER ipam_pools_insert_guard
BEFORE INSERT ON ipam_pools
BEGIN
    SELECT RAISE(ABORT, 'IPAM_ID_ALREADY_USED')
    WHERE EXISTS (SELECT 1 FROM ipam_resource_ids r
                  WHERE r.kind = 'POOL' AND r.tenant_id = NEW.tenant_id AND r.space_id = NEW.space_id AND r.id = NEW.id);
    SELECT RAISE(ABORT, 'IPAM_POOL_OUTSIDE_SUBNET')
    WHERE NOT EXISTS (
        SELECT 1 FROM ipam_prefixes p
        WHERE p.tenant_id = NEW.tenant_id AND p.space_id = NEW.space_id AND p.id = NEW.subnet_id
          AND p.kind = 'SUBNET' AND p.usage = 'STANDARD' AND p.family = NEW.family
          AND (CASE NEW.family WHEN 4 THEN p.first_ip < NEW.first_ip AND p.last_ip > NEW.last_ip
               ELSE p.first_ip < NEW.first_ip AND p.last_ip >= NEW.last_ip END)
    );
    SELECT RAISE(ABORT, 'IPAM_POOL_OVERLAP')
    WHERE EXISTS (
        SELECT 1 FROM ipam_pools p
        WHERE p.tenant_id = NEW.tenant_id AND p.space_id = NEW.space_id AND p.subnet_id = NEW.subnet_id
          AND p.first_ip <= NEW.last_ip AND p.last_ip >= NEW.first_ip
    );
    SELECT RAISE(ABORT, 'IPAM_DHCP_POOL_OCCUPIED')
    WHERE NEW.pool_kind = 'DHCP' AND EXISTS (
        SELECT 1 FROM ipam_occupancies a
        WHERE a.tenant_id = NEW.tenant_id AND a.space_id = NEW.space_id AND a.subnet_id = NEW.subnet_id
          AND a.address BETWEEN NEW.first_ip AND NEW.last_ip
    );
END;

CREATE TRIGGER ipam_pools_record_id
AFTER INSERT ON ipam_pools
BEGIN
    INSERT INTO ipam_resource_ids VALUES ('POOL', NEW.tenant_id, NEW.space_id, NEW.id);
END;

CREATE TRIGGER ipam_pools_shape_immutable
BEFORE UPDATE OF tenant_id, space_id, id, subnet_id, family, pool_kind, first_ip, last_ip ON ipam_pools
WHEN NEW.tenant_id IS NOT OLD.tenant_id OR NEW.space_id IS NOT OLD.space_id OR NEW.id IS NOT OLD.id
  OR NEW.subnet_id IS NOT OLD.subnet_id OR NEW.family IS NOT OLD.family OR NEW.pool_kind IS NOT OLD.pool_kind
  OR NEW.first_ip IS NOT OLD.first_ip OR NEW.last_ip IS NOT OLD.last_ip
BEGIN
    SELECT RAISE(ABORT, 'IPAM_POOL_SHAPE_IMMUTABLE');
END;

CREATE TRIGGER ipam_occupancies_insert_guard
BEFORE INSERT ON ipam_occupancies
BEGIN
    SELECT RAISE(ABORT, 'IPAM_ID_ALREADY_USED')
    WHERE EXISTS (SELECT 1 FROM ipam_resource_ids r
                  WHERE r.kind = 'OCCUPANCY' AND r.tenant_id = NEW.tenant_id AND r.space_id = NEW.space_id AND r.id = NEW.id);
    -- 即使外层 SQL 误用 OR REPLACE，也不能删除占用者。UNIQUE 仍是最终索引约束。
    SELECT RAISE(ABORT, 'IPAM_ADDRESS_OCCUPIED')
    WHERE EXISTS (SELECT 1 FROM ipam_occupancies a
                  WHERE a.tenant_id = NEW.tenant_id AND a.space_id = NEW.space_id
                    AND a.family = NEW.family AND a.address = NEW.address);
    SELECT RAISE(ABORT, 'IPAM_ADDRESS_OUTSIDE_SUBNET')
    WHERE NOT EXISTS (
        SELECT 1 FROM ipam_prefixes p
        WHERE p.tenant_id = NEW.tenant_id AND p.space_id = NEW.space_id AND p.id = NEW.subnet_id
          AND p.kind = 'SUBNET' AND p.family = NEW.family
          AND NEW.address BETWEEN p.first_ip AND p.last_ip
          AND (p.usage IN ('P2P', 'HOST') OR
               (NEW.address <> p.first_ip AND (NEW.family = 6 OR NEW.address <> p.last_ip)))
    );
    SELECT RAISE(ABORT, 'IPAM_ADDRESS_DHCP_MANAGED')
    WHERE EXISTS (
        SELECT 1 FROM ipam_pools p
        WHERE p.tenant_id = NEW.tenant_id AND p.space_id = NEW.space_id AND p.subnet_id = NEW.subnet_id
          AND p.pool_kind = 'DHCP' AND NEW.address BETWEEN p.first_ip AND p.last_ip
    );
END;

CREATE TRIGGER ipam_occupancies_record_id
AFTER INSERT ON ipam_occupancies
BEGIN
    INSERT INTO ipam_resource_ids VALUES ('OCCUPANCY', NEW.tenant_id, NEW.space_id, NEW.id);
END;

CREATE TRIGGER ipam_occupancies_identity_immutable
BEFORE UPDATE OF tenant_id, space_id, id, subnet_id, family, address, created_at_ms ON ipam_occupancies
WHEN NEW.tenant_id IS NOT OLD.tenant_id OR NEW.space_id IS NOT OLD.space_id OR NEW.id IS NOT OLD.id
  OR NEW.subnet_id IS NOT OLD.subnet_id OR NEW.family IS NOT OLD.family OR NEW.address IS NOT OLD.address
  OR NEW.created_at_ms IS NOT OLD.created_at_ms
BEGIN
    SELECT RAISE(ABORT, 'IPAM_ADDRESS_IDENTITY_IMMUTABLE');
END;

-- 确认 HOLD 的请求还必须使用事务捕获的可信时间作到期判定；
-- 本触发器不读取操作系统时钟，不把权限或到期验证移入 SQL。
CREATE TRIGGER ipam_occupancies_transition_guard
BEFORE UPDATE ON ipam_occupancies
WHEN NEW.tenant_id IS OLD.tenant_id AND NEW.space_id IS OLD.space_id AND NEW.id IS OLD.id
 AND NEW.subnet_id IS OLD.subnet_id AND NEW.family IS OLD.family AND NEW.address IS OLD.address
 AND NEW.created_at_ms IS OLD.created_at_ms
 AND NOT (OLD.kind = 'HOLD' AND NEW.kind = 'STATIC' AND NEW.expires_at_ms IS NULL
          AND NEW.owner_ref IS OLD.owner_ref AND OLD.revision < 9223372036854775807
          AND NEW.revision = OLD.revision + 1)
BEGIN
    SELECT RAISE(ABORT, 'IPAM_OCCUPANCY_TRANSITION_INVALID');
END;

CREATE TRIGGER ipam_tenants_insert_guard
BEFORE INSERT ON ipam_tenants
BEGIN
    SELECT RAISE(ABORT, 'IPAM_ID_ALREADY_USED')
    WHERE EXISTS (SELECT 1 FROM ipam_resource_ids r WHERE r.kind = 'TENANT' AND r.tenant_id = '' AND r.space_id = '' AND r.id = NEW.id);
    SELECT RAISE(ABORT, 'IPAM_SLUG_CONFLICT')
    WHERE EXISTS (SELECT 1 FROM ipam_tenants t WHERE t.slug = NEW.slug);
END;

CREATE TRIGGER ipam_tenants_record_id
AFTER INSERT ON ipam_tenants
BEGIN
    INSERT INTO ipam_resource_ids VALUES ('TENANT', '', '', NEW.id);
END;

CREATE TRIGGER ipam_tenants_update_guard
BEFORE UPDATE ON ipam_tenants
BEGIN
    SELECT RAISE(ABORT, 'IPAM_ID_IMMUTABLE') WHERE NEW.id IS NOT OLD.id;
    SELECT RAISE(ABORT, 'IPAM_REVISION_INVALID')
    WHERE OLD.revision = 9223372036854775807 OR NEW.revision <> OLD.revision + 1;
    SELECT RAISE(ABORT, 'IPAM_SLUG_CONFLICT')
    WHERE EXISTS (SELECT 1 FROM ipam_tenants t WHERE t.slug = NEW.slug AND t.id <> OLD.id);
END;

CREATE TRIGGER ipam_tenants_delete_guard
BEFORE DELETE ON ipam_tenants
WHEN EXISTS (SELECT 1 FROM ipam_spaces s WHERE s.tenant_id = OLD.id)
BEGIN
    SELECT RAISE(ABORT, 'IPAM_SCOPE_REFERENCED');
END;

CREATE TRIGGER ipam_spaces_insert_guard
BEFORE INSERT ON ipam_spaces
BEGIN
    SELECT RAISE(ABORT, 'IPAM_ID_ALREADY_USED')
    WHERE EXISTS (SELECT 1 FROM ipam_resource_ids r WHERE r.kind = 'SPACE' AND r.tenant_id = NEW.tenant_id AND r.space_id = '' AND r.id = NEW.id);
    SELECT RAISE(ABORT, 'IPAM_SLUG_CONFLICT')
    WHERE EXISTS (SELECT 1 FROM ipam_spaces s WHERE s.tenant_id = NEW.tenant_id AND s.slug = NEW.slug);
END;

CREATE TRIGGER ipam_spaces_record_id
AFTER INSERT ON ipam_spaces
BEGIN
    INSERT INTO ipam_resource_ids VALUES ('SPACE', NEW.tenant_id, '', NEW.id);
END;

CREATE TRIGGER ipam_spaces_update_guard
BEFORE UPDATE ON ipam_spaces
BEGIN
    SELECT RAISE(ABORT, 'IPAM_ID_IMMUTABLE') WHERE NEW.tenant_id IS NOT OLD.tenant_id OR NEW.id IS NOT OLD.id;
    SELECT RAISE(ABORT, 'IPAM_REVISION_INVALID')
    WHERE OLD.revision = 9223372036854775807 OR NEW.revision <> OLD.revision + 1;
    SELECT RAISE(ABORT, 'IPAM_SLUG_CONFLICT')
    WHERE EXISTS (SELECT 1 FROM ipam_spaces s WHERE s.tenant_id = NEW.tenant_id AND s.slug = NEW.slug AND s.id <> OLD.id);
END;

CREATE TRIGGER ipam_spaces_delete_guard
BEFORE DELETE ON ipam_spaces
WHEN EXISTS (SELECT 1 FROM ipam_prefixes p WHERE p.tenant_id = OLD.tenant_id AND p.space_id = OLD.id)
BEGIN
    SELECT RAISE(ABORT, 'IPAM_SCOPE_REFERENCED');
END;

CREATE TRIGGER ipam_prefixes_delete_guard
BEFORE DELETE ON ipam_prefixes
WHEN EXISTS (SELECT 1 FROM ipam_prefixes p
             WHERE p.tenant_id = OLD.tenant_id AND p.space_id = OLD.space_id AND p.parent_id = OLD.id)
  OR EXISTS (SELECT 1 FROM ipam_pools p
             WHERE p.tenant_id = OLD.tenant_id AND p.space_id = OLD.space_id AND p.subnet_id = OLD.id)
  OR EXISTS (SELECT 1 FROM ipam_occupancies a
             WHERE a.tenant_id = OLD.tenant_id AND a.space_id = OLD.space_id AND a.subnet_id = OLD.id)
BEGIN
    SELECT RAISE(ABORT, 'IPAM_PREFIX_REFERENCED');
END;

CREATE TRIGGER ipam_prefixes_revision_guard
BEFORE UPDATE ON ipam_prefixes
WHEN NEW.tenant_id IS OLD.tenant_id AND NEW.space_id IS OLD.space_id AND NEW.id IS OLD.id
 AND NEW.parent_id IS OLD.parent_id AND NEW.family IS OLD.family AND NEW.prefix_len IS OLD.prefix_len
 AND NEW.first_ip IS OLD.first_ip AND NEW.last_ip IS OLD.last_ip AND NEW.kind IS OLD.kind AND NEW.usage IS OLD.usage
 AND (OLD.revision = 9223372036854775807 OR NEW.revision <> OLD.revision + 1)
BEGIN
    SELECT RAISE(ABORT, 'IPAM_REVISION_INVALID');
END;

CREATE TRIGGER ipam_pools_revision_guard
BEFORE UPDATE ON ipam_pools
WHEN NEW.tenant_id IS OLD.tenant_id AND NEW.space_id IS OLD.space_id AND NEW.id IS OLD.id
 AND NEW.subnet_id IS OLD.subnet_id AND NEW.family IS OLD.family AND NEW.pool_kind IS OLD.pool_kind
 AND NEW.first_ip IS OLD.first_ip AND NEW.last_ip IS OLD.last_ip
 AND (OLD.revision = 9223372036854775807 OR NEW.revision <> OLD.revision + 1)
BEGIN
    SELECT RAISE(ABORT, 'IPAM_REVISION_INVALID');
END;

COMMIT;
