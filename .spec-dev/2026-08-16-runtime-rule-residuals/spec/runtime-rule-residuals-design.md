---
# —— spec-dev 漂移守卫锚点（机器可校验，勿删）——
spec_dev:
  version: 1
  feature: runtime-rule-residuals
  status: active
  covers:
    - "sources.yaml"
    - "config/passwall2-groups.yaml"
    - "custom/geosite/**"
    - "custom/geoip/**"
    - "internal/app/**"
    - "internal/config/**"
    - "internal/geodat/**"
    - "internal/geoip/**"
    - "internal/geosite/**"
    - "internal/manifest/**"
    - "internal/model/**"
    - "internal/passwall/**"
    - "internal/rules/**"
    - "internal/shadowrocket/**"
    - "internal/targets/**"
    - "internal/verify/**"
    - "internal/workspace/**"
    - "README.md"
    - "context.md"
  sync_commit: 4081ec95ffbbdd4b1f5267f4fca459981c3be1c4
  supersedes:
    - ".spec-dev/2026-08-15-shadowrocket-conf/spec/shadowrocket-conf-design.md"
  superseded_by: null
---

# 运行时规则残差优化 设计

## 背景与目标

PassWall2 加载托管组、Shadowrocket 展开胖配置时，会重复装入大量已被前序规则或等价出口覆盖的域名规则。最初通过新增 namespaced 残差标签降低运行集合，但完整公共标签和残差副本同时进入最终 DAT，使 `geosite.dat` 明显增大。本特性改为从中间编译结果生成经过保守证明的残差，并原位覆写最终 `proxy`、`direct`，同时安全迁移现有 PassWall2 Shunt 节点配置。

**成功标准 / Success criteria**：相同输入下，GFW/proxy/tld-not-cn 合并规则数至少下降 30%，全部活动域名规则数、Shadowrocket 的 GeoSite 派生规则行数/字节数及最终 `geosite.dat` 字节数严格下降；因新增内联 `geoip:google`，`shadowrocket.conf` 总字节允许增长但不得超过基线的 105%，且其他部分不得产生净增长；最终 GeoSite DAT 仅含原位残差 `proxy`、`direct`，不含 `gfw`、`tld-not-cn` 或旧 `crs-*` 残差标签；最终 GeoIP DAT 只含 `cn`、`google`、`netflix`、`private`、`telegram`；`reject` 保持兼容。在声明的兼容 Shunt 配置下，Xray `AsIs`、Xray `IPIfNonMatch` 和 sing-box 原生路由的固定语料最终出口不变，三种运行矩阵的稳定 RSS 中位数均至少下降 5%。

## 非目标

- 保持最终 `gfw`、`tld-not-cn`、`crs-proxy-residual`、`crs-direct-residual` 标签兼容
- 保持最终 `proxy`、`direct` 与上游完整标签规则指纹一致
- 保持未被当前托管组或来源引用的 GeoIP 国家/特殊标签兼容
- 改写 `reject` 的来源、内容、优先级或出口
- 通过 GeoIP 规则证明某条域名规则冗余
- 支持 Xray `IPOnDemand` 的本轮设备验收
- 改变八个 Release 资产的闭集或增加性能报告资产
- 完成 iPhone 真机导入；该项继续 DEFERRED

## 术语表

- **中间基础标签 / intermediate basis tag**：由来源、community 与 custom 合并并编入中间 DAT、供残差推导使用的标签，包括 `gfw`、`proxy`、`tld-not-cn`、`direct`；不承诺在最终 DAT 保留。
- **原位残差标签 / in-place residual tag**：从编译后 DAT 中保守删除不可达或等价覆盖规则后，覆写同名最终 `proxy` 或 `direct` 的 GeoSite 标签。_Avoid_：兼容标签、附加标签
- **覆盖证明 / coverage proof**：单条规则足以证明候选规则的全部匹配集合已由另一条规则接管的保守判定；不做多规则并集或正则语言包含推断。_Avoid_：模糊去重、近似匹配
- **Shunt 节点 / Shunt node**：PassWall2 `nodes` 中 `protocol='_shunt'` 的节点配置；它按 `crs_<group-id>` 及辅助字段保存每组出口。_Avoid_：`shunt_rules` 规则组
- **UCI 配置族 / UCI option family**：某规则组在 Shunt 节点上的主出口、`_fakedns` 与 `_proxy_tag` 三个选项。_Avoid_：单一出口字段
- **路由等价语料 / routing equivalence corpus**：覆盖最终保留的 13 组、合并后的 GFW/proxy/tld-not-cn 规则、未命中域名与 GeoIP 二次匹配的固定请求集合，用于优化前后设备对照；基线命中 `gfw`、`proxy` 或 `tld_not_cn` 均映射为候选 `proxy` 后比较最终出口，不包含已批准删除的 BilibiliHMT 契约。
- **基线发布物 / baseline artifact**：本特性代码改动前，用与候选相同的公共来源响应快照、community、custom 与构建参数生成的八资产。_Avoid_：任意历史 latest、不同上游输入
- **活动域名规则数 / active domain rule count**：按默认托管组展开最终 GeoSite 标签、组内按 kind/value/attributes 精确去重后逐组求和的规则数；跨组重复仍分别计数。_Avoid_：DAT 全标签总量、文件字节数
- **GFW/proxy/tld-not-cn 合并规则数 / combined proxy rule count**：基线 `gfw`、`proxy` 与 `tld_not_cn` 三个活动组的规则数之和，与候选最终唯一 `proxy` 标签的规则数比较；沿用活动域名规则数的组内去重口径。_Avoid_：仅计算裁剪前中间标签
- **兼容 Shunt 配置 / compatible Shunt configuration**：`gfw`、`proxy`、`tld_not_cn` 三组主出口均已启用且相同，三个 UCI 配置族逐项相同或辅助项同时缺失的节点配置；本仓当前目标配置为三组均 `_default`。_Avoid_：任意历史节点、单侧关闭配置

## 影响面

- 构建流水线增加中间 GeoSite 编译、无损 DAT 解码、残差推导与最终重编译
- 默认 PassWall2 组由 16 个改为 13 个，并改变 `proxy`、`direct` 的 GeoSite 引用
- PassWall2 安装器增加 Shunt 节点预检、迁移、冲突失败和回滚
- Shadowrocket 策略组由 16 个改为 13 个，`tld-not-cn` 规则直接归入唯一 `proxy` 策略组
- 删除 BilibiliHMT 的远程来源、双侧标签、组、模板、测试夹具与文档
- manifest 与仓库契约要求最终 `proxy`、`direct` 存在，并禁止中间/旧残差标签进入最终 DAT
- GeoIP 编译输出按 manifest 所需标签闭集投影，Google 组新增 `geoip:google` 引用
- 不引入新的第三方依赖；共享现有 DAT wire 解码能力并补齐 regexp 与 attributes

## 已确认的关键决策

- 选择“编译后 DAT 保守残差”而不是源文本过滤或平台渲染期过滤：include、属性和规则类型语义只在编译后输入上完整可见，且一份派生结果可同时服务 PassWall2 与 Shadowrocket
- 选择原位覆写 `proxy`、`direct` 并删除中间/旧残差标签，不再保留外部标签兼容（详见 `../../adr/0004-in-place-residual-tags.md`）
- `gfw` 托管组并入既有 `proxy`，`proxy` 继续作为唯一规范组 ID；不创建或改名 `crs_proxy`、`crs_direct`
- 完整删除 BilibiliHMT：删除来源、GeoSite/GeoIP 标签、组、模板、测试和文档，不保留兼容空壳
- PassWall2 对全部已有 Shunt 节点做事务化迁移：缺失值按非空一侧归一化、相同值合并、冲突值阻断安装；单侧缺失会有意启用合并全集，不属于路由等价承诺
- `proxy` 默认组只引用最终 `proxy`；其内容是裁剪后的 `gfw ∪ proxy` 与完整 `tld-not-cn` 的精确去重并集。PassWall2 安装前校验历史 `proxy`/`tld_not_cn` 配置再迁入唯一 `proxy`，Shadowrocket 不再生成独立别名组
- `private`、`cn`、`direct` 保持直连配置，不由安装器重写
- GeoIP 使用完整底包作为合并输入，但最终只发布 manifest 所需的 `cn`、`google`、`netflix`、`private`、`telegram`；其他标签不保留兼容
- 设备验收覆盖 Xray `AsIs`、Xray `IPIfNonMatch` 与 sing-box 原生路由；本轮不扩大到 `IPOnDemand`

## 取代与共存

- [部分取代] `.spec-dev/2026-08-15-shadowrocket-conf/spec/shadowrocket-conf-design.md`：Requirement「十六个具名策略组与默认出口」——默认托管组改为 13 个，删除 GFW/BilibiliHMT/独立 `tld_not_cn` 组，并把 `tld-not-cn` 规则实体归入最终 `proxy`
- [部分取代] 同一旧 spec 的成功标准、非目标“不得修改 PassWall2/UCI/DAT 语义”、关键决策“16 个具名 select 组”和风险说明“组间不去重”——本特性明确修改这些契约；八资产闭集、从最终 DAT 展开、规则类型映射、尾规则和真机 DEFERRED 与旧 spec 分面共存
- [分面共存] `.spec-dev/adr/0001-shadowrocket-fat-conf.md`：仍使用单份胖 conf，不改变交付形态
- [分面共存] `.spec-dev/adr/0002-expand-from-compiled-dat.md`：Shadowrocket 仍从最终 DAT 展开；本特性在最终 DAT 之前增加残差派生与标签原位覆写
- [取代] `.spec-dev/adr/0003-compiled-dat-residual-tags.md`：不再新增 namespaced 残差标签或保留完整公共标签，改由 ADR-0004 定义原位残差

## ADDED Requirements

### Requirement: 从编译后 DAT 原位生成 proxy/direct

完整构建 SHALL 从中间编译的 GeoSite DAT 生成非空的 `proxy` 与 `direct` 残差，原位覆写合并目录中的同名标签，再编译最终 `geosite.dat`。

#### Scenario: include 与属性在残差输入中已展开

- **GIVEN** 基础标签通过 `include:` 获得域名，且至少一条规则带 attributes
- **WHEN** 执行完整构建
- **THEN** 残差推导看到展开后的规则类型、值与完整 attributes，最终 DAT 同时含两个非空的原位残差标签

#### Scenario: skip-compile 不生成残差发布物

- **GIVEN** 构建使用 `--skip-compile`
- **WHEN** prepare 阶段完成
- **THEN** 构建不运行中间编译、残差推导或正式 publish 切换

### Requirement: proxy 残差只删除可证明的冗余规则

最终 `proxy` SHALL 等于 `gfw ∪ proxy` 精确去重后，仅删除被更早活动域名组覆盖或被 `tld-not-cn` 证明等价覆盖的规则，再与完整 `tld-not-cn` 精确去重合并。`tld-not-cn` 的规则实体因此全部由最终 `proxy` 承载。

#### Scenario: GFW 与 proxy 合并且服务组优先

- **GIVEN** 一条规则同时存在于 `gfw` 和 `proxy`，另一条 proxy 规则已被更早的 YouTube 规则覆盖
- **WHEN** 推导 proxy 残差
- **THEN** 重复规则只保留一次，YouTube 已覆盖规则不进入残差，未被证明覆盖的规则仍保留

#### Scenario: 不借用后序直连组裁剪 proxy

- **GIVEN** 一条 proxy 规则也能被后序 `cn` 或 `direct` 匹配，但不能被 `tld-not-cn` 证明覆盖
- **WHEN** 推导 proxy 残差
- **THEN** 该规则保留在最终 `proxy`，避免出口从代理变为直连

#### Scenario: tld-not-cn 实体并入 proxy

- **GIVEN** `tld-not-cn` 含 proxy basis 中不存在的规则，且也含与裁剪后 proxy 重复的规则
- **WHEN** 生成最终 `proxy`
- **THEN** 独有规则写入最终 `proxy`，重复规则只保留一次，最终 DAT 不再含 `tld-not-cn`

### Requirement: direct 残差只删除前序不可达规则

最终 `direct` SHALL 从中间完整 `direct` 中仅删除被最终活动顺序中更早域名规则单条覆盖的规则，并原位覆写同名标签。

#### Scenario: tld-not-cn 与 cn 使 direct 规则不可达

- **GIVEN** direct 中分别存在可由前序 `tld-not-cn` 和前序 `cn` 域名规则证明覆盖的规则
- **WHEN** 推导 direct 残差
- **THEN** 两条不可达规则不进入残差，其他无法证明的 direct 规则保留

#### Scenario: GeoIP 不作为域名覆盖证据

- **GIVEN** 某 direct 域名解析到 `geoip:cn`，但没有任何前序域名规则覆盖它
- **WHEN** 推导 direct 残差
- **THEN** 该域名规则保留，不因解析结果被裁剪

### Requirement: 覆盖证明保持保守和确定

覆盖证明 SHALL 仅在 attributes 规范化集合相同，且候选满足“full 被相同 full、标签边界正确的相同/父级 suffix 或确定包含的 keyword 覆盖”“suffix 被标签边界正确的相同/父级 suffix 或确定包含的 keyword 覆盖”“keyword 被其子串 keyword 覆盖”“regexp 文本完全相同”之一时删除候选，否则保留。

#### Scenario: suffix 标签边界不产生假覆盖

- **GIVEN** 已有 suffix `ample.com`，候选为 `example.com`
- **WHEN** 执行覆盖证明
- **THEN** 候选不被判为已覆盖

#### Scenario: 正则只做同类等值去重

- **GIVEN** 已有与候选语义看似相近但文本不同的 regexp
- **WHEN** 执行覆盖证明
- **THEN** 候选保留；只有 regexp 文本与 attributes 均相同时才可去重

#### Scenario: 输出顺序可重现

- **GIVEN** 相同规则集合以不同输入顺序提供
- **WHEN** 两次推导残差
- **THEN** 输出按 kind、value、attributes 的稳定顺序产生完全相同字节

#### Scenario: 最终 GeoSite 编译输出可重现

- **GIVEN** 相同标签与规则集合被编译器以不同 tag、rule 或 attribute 顺序写出
- **WHEN** 构建准备发布最终 `geosite.dat`
- **THEN** 发布前按 tag、kind、value、attributes 规范化，重复构建产生完全相同字节且语义不变

### Requirement: 不安全的残差构建不得发布

完整构建 SHALL 在中间基础标签缺失、残差为空、DAT 解码丢失类型或属性、规范化遇到未知 protobuf 字段、原位覆写/删除失败、以及最终标签验证失败时终止并保留旧 publish。

#### Scenario: 中间基础标签缺失

- **GIVEN** 中间 DAT 缺少 `gfw`、`proxy`、`tld-not-cn` 或 `direct` 任一基础标签
- **WHEN** 完整构建尝试推导原位残差
- **THEN** 构建在最终编译前失败并指出缺失标签，旧 publish 不变

#### Scenario: 未知 GeoSite 字段不被规范化吞掉

- **GIVEN** 编译器输出在 document、tag 或 domain 层出现当前模型不认识的 protobuf 字段
- **WHEN** 构建规范化最终 `geosite.dat`
- **THEN** 构建明确失败且旧 publish 不变，不得静默删除未知字段后继续发布

#### Scenario: 中间 DAT 不污染最终探测缓存

- **GIVEN** 中间 DAT 仍含完整基础标签而最终 DAT 应含原位残差
- **WHEN** 最终 required/forbidden tag 校验运行
- **THEN** 校验读取最终 DAT 的新鲜视图并成功看到 `proxy`、`direct`，且看不到 `gfw`、`tld-not-cn` 与两个旧 `crs-*` 标签

#### Scenario: 无法回写的 typed attribute 失败

- **GIVEN** 中间 DAT 含 bool true 以外的 attribute typed value 或未知 attribute wire field
- **WHEN** 无损解码准备生成 domain-list 文本
- **THEN** 构建失败并指出 tag、规则与属性，不丢弃或降级该属性

### Requirement: 最终 DAT 删除中间和旧残差标签

最终 DAT SHALL 要求 `proxy`、`direct` 存在，并禁止 `gfw`、`tld-not-cn`、`crs-proxy-residual`、`crs-direct-residual` 存在；不提供这些已删除标签的兼容空壳。

#### Scenario: 原位残差替换完整标签

- **GIVEN** 默认仓库配置执行完整构建
- **WHEN** 展开最终 GeoSite 标签集合
- **THEN** `proxy`、`direct` 为非空原位残差，四个禁止标签均不存在

### Requirement: reject 规则保持完全不变

相同构建输入下，最终 `reject` 标签 SHALL 保持规则类型、值、attributes 与顺序指纹不变。

#### Scenario: reject 指纹不受优化影响

- **GIVEN** 同一份锁定来源分别运行优化前和优化后构建
- **WHEN** 无损展开最终 `reject`
- **THEN** 两次规则指纹完全相同，且默认组顺序仍以 reject 为第一组

### Requirement: 最终 GeoIP 只发布活动标签闭集

完整构建 SHALL 在合并底包与自定义 CIDR 后，仅把 manifest 的 required GeoIP 标签写入最终 `geoip.dat`。默认配置的闭集 SHALL 精确为 `cn`、`google`、`netflix`、`private`、`telegram`，不得保留其他国家或特殊标签。

#### Scenario: 完整底包被投影为五个活动标签

- **GIVEN** GeoIP 底包除五个活动标签外还含 `us` 等未引用标签
- **WHEN** 执行完整构建并解码最终 `geoip.dat`
- **THEN** 标签集合精确等于 `cn`、`google`、`netflix`、`private`、`telegram`，`us` 不存在

#### Scenario: Google IP 进入双端规则

- **GIVEN** 底包含非空 `geoip:google`，默认 Google 组同时引用 GeoSite/GeoIP `google`
- **WHEN** 渲染 PassWall2 fragment 与 Shadowrocket conf
- **THEN** `crs_google` 包含 `geoip:google`，Shadowrocket 在 Google 服务策略下展开对应 IP-CIDR 规则

### Requirement: PassWall2 按规范合并 GFW 节点配置

安装器 SHALL 对每个已有 Shunt 节点的主出口、`_fakedns` 与 `_proxy_tag` 三个 UCI 配置族执行 `gfw → proxy` 的缺失迁移、相同合并和冲突拒绝。

#### Scenario: proxy 缺失时继承 gfw

- **GIVEN** 某 Shunt 节点有 `crs_gfw='_default'` 及其辅助值、缺少对应 `crs_proxy*`，且 `crs_tld_not_cn*` 与迁移后的 proxy 逐项等价
- **WHEN** 安装器执行迁移
- **THEN** 三个值迁入对应 `crs_proxy*`，随后旧 `crs_gfw*` 被删除

#### Scenario: 单侧关闭按非空一侧归一化

- **GIVEN** 某 Shunt 节点只启用了 gfw 或只启用了 proxy，且合并后的 proxy 与 `tld_not_cn` 满足等价预检
- **WHEN** 安装器合并两个旧组
- **THEN** 合并后的 `crs_proxy*` 采用非空一侧配置并承载 `gfw ∪ proxy ∪ tld-not-cn`，安装日志明确提示该节点从单侧关闭归一化为合并全集启用

#### Scenario: gfw 与 proxy 冲突时零修改失败

- **GIVEN** 任一 Shunt 节点的同一配置族中 `crs_gfw*` 与 `crs_proxy*` 均非空且不同
- **WHEN** 安装器执行预检
- **THEN** 安装器在修改 live UCI 前失败，输出节点 section 与冲突字段，全部节点保持原样

#### Scenario: 重复安装保持幂等

- **GIVEN** 一次安装已成功迁移并删除旧字段
- **WHEN** 再次运行同一安装器
- **THEN** Shunt 节点与托管规则配置不产生新的变化

### Requirement: PassWall2 迁移前强制旧 proxy 与 tld-not-cn 语义一致

安装器 SHALL 在所有仍含 `crs_gfw*` 或 `crs_tld_not_cn*` 历史字段的 Shunt 节点上，先按既有规则归一 `crs_gfw* → crs_proxy*`，再验证所得 `crs_proxy*` 与 `crs_tld_not_cn*` 三个 UCI 配置族逐项等价；缺失与非缺失不视为相同。仅含规范 `crs_proxy*` 的节点 SHALL 直接视为已迁移状态。

#### Scenario: 三个旧组均继承 Default

- **GIVEN** `crs_gfw`、`crs_proxy` 与 `crs_tld_not_cn` 归一后都为 `_default`，对应辅助字段也逐项相同或均缺失
- **WHEN** 安装器执行兼容性预检
- **THEN** 预检通过，迁移后只保留 `crs_proxy='_default'` 及对应辅助字段

#### Scenario: 等价出口条件不成立

- **GIVEN** 某 Shunt 节点把 proxy 与 tld-not-cn 指向不同出口，或只有一侧启用 FakeDNS/proxy tag
- **WHEN** 安装器执行兼容性预检
- **THEN** 安装被拒绝且 live UCI、URL 与 DAT 均不修改

#### Scenario: 规范 proxy-only 配置重复安装

- **GIVEN** 节点只含上次成功迁移留下的 `crs_proxy*`，不再含 `crs_gfw*` 或 `crs_tld_not_cn*`
- **WHEN** 再次执行安装器预检与迁移
- **THEN** 预检通过且节点字段不产生变化

### Requirement: 安装器事务包含节点迁移

安装器 SHALL 把 Shunt 节点迁移、GFW/tld-not-cn/BilibiliHMT 字段清理、托管 `shunt_rules` 更新和全局 DAT URL 更新纳入同一可观察事务。

#### Scenario: 后续步骤失败时恢复节点

- **GIVEN** 节点预检通过且迁移已暂存，但规则更新或提交阶段失败
- **WHEN** 安装器进入回滚
- **THEN** 节点字段、托管规则与全局 URL 均恢复安装前值，并输出回滚结果

### Requirement: 安装器清理全部废弃节点字段

预检通过后的成功安装 SHALL 从每个 Shunt 节点删除 `crs_gfw*`、`crs_tld_not_cn*` 与 `crs_bilibili_hmt*` 三个 UCI 配置族。

#### Scenario: 成功安装后无废弃字段

- **GIVEN** 多个 Shunt 节点分别保存 GFW、tld-not-cn 和 BilibiliHMT 的主出口、FakeDNS 或 proxy tag 字段
- **WHEN** 安装器成功提交
- **THEN** `uci show passwall2` 中不再出现任何对应 `crs_gfw*`、`crs_tld_not_cn*` 或 `crs_bilibili_hmt*` 字段

### Requirement: 安装器保留无关节点语义

安装器 SHALL 保持 Shunt 节点的 `default_node`、reject 及 `private`、`cn`、`direct` 配置族与安装前完全相同。

#### Scenario: 默认代理和三组直连不被重写

- **GIVEN** Shunt 节点的 Default 为代理，reject 保持原配置，`private`、`cn`、`direct` 主出口均为 `_direct`
- **WHEN** 安装器成功完成规则更新
- **THEN** 上述字段及其辅助字段逐项等于安装前快照

### Requirement: custom 输入参与原位残差

仓库目标契约 SHALL 允许 `proxy`、`direct`、`gfw` 与 `tld-not-cn` 接受现有 source、community 或 custom 输入；这些输入先编入中间 DAT，再按原位残差契约生成最终 `proxy`、`direct`。`gfw` 与 `tld-not-cn` custom 目标不意味着同名最终标签存在。

#### Scenario: proxy custom 先合并再裁剪

- **GIVEN** 用户向 `custom/geosite/proxy.yaml` 增加有效规则
- **WHEN** 运行 repository/custom contract 测试
- **THEN** 输入被中间 `proxy` 接受，并按与其他 proxy basis 相同的覆盖证明进入或移出最终 `proxy`

### Requirement: BilibiliHMT 不得重新进入发布契约

仓库校验 SHALL 拒绝 `xiaolin-bilibili`、`bilibili_hmt` 或双侧 `BilibiliHMT` 标签重新出现在来源、组、custom 或最终 DAT 中。

#### Scenario: 最终 DAT 不含 BilibiliHMT

- **GIVEN** 默认仓库配置执行完整构建
- **WHEN** 校验 required 与 forbidden tags
- **THEN** geosite.dat 与 geoip.dat 均不含 `BilibiliHMT`，且配置、模板和文档没有残留引用

### Requirement: 优化前后路由决策等价

候选发布物 SHALL 在兼容 Shunt 配置下对保留的 13 组及路由等价语料保持与基线发布物相同的最终出口；基线命中 `gfw`、`proxy` 或 `tld_not_cn` 映射为候选 `proxy` 后比较，BilibiliHMT 的显式删除除外。

#### Scenario: Xray AsIs 域名路由等价

- **GIVEN** 同一 PassWall2 Shunt 节点、相同 Default/Direct/Reject 出口和固定语料，`domainStrategy='AsIs'`
- **WHEN** 分别安装基线与候选并发送只依赖域名规则的请求
- **THEN** 每个请求的命中组与最终出口一致，候选不依赖 GeoIP 才得到正确结果

#### Scenario: Xray IPIfNonMatch 二次 IP 匹配等价

- **GIVEN** 同一配置改为 `domainStrategy='IPIfNonMatch'`，语料含域名未命中后落入 private/cn GeoIP 的请求
- **WHEN** 分别运行基线与候选
- **THEN** 域名首轮与解析 IP 后的二次匹配决策均一致

#### Scenario: sing-box 原生路由等价

- **GIVEN** PassWall2 使用 sing-box Shunt 节点及相同语料
- **WHEN** 分别运行基线与候选
- **THEN** 每个请求的命中规则与最终出口一致，不把 Xray 的 `domainStrategy` 名称套用于 sing-box

### Requirement: 代理规则显著下降且 Shadowrocket 增长受控

候选发布物 SHALL 在相同输入下使 GFW/proxy/tld-not-cn 合并规则数不高于基线的 70%，并使全部活动域名规则数、Shadowrocket 中 GeoSite 派生规则行数/字节数分别严格小于基线。`shadowrocket.conf` 总字节 MAY 因新增内联 `geoip:google` 增长，但 SHALL 不高于基线的 105%；除 Google IP 规则块外的候选字节数 SHALL 严格小于基线对应部分。

#### Scenario: 代理规则减少至少 30% 且 Google IP 增长受控

- **GIVEN** 基线与候选使用同一来源快照和完整 DAT 输入
- **WHEN** 分别统计 PassWall2 托管组实际引用的 GeoSite 规则、Shadowrocket 的 GeoSite/Google IP 规则块及总字节
- **THEN** 候选的 GFW/proxy/tld-not-cn 合并规则数不超过基线的 70%
- **AND** 候选的活动域名规则总数、Shadowrocket GeoSite 派生规则行数和字节数分别严格小于基线
- **AND** 候选 `shadowrocket.conf` 总字节数不超过基线的 105%，移除新增 Google IP 规则块后严格小于基线对应部分

### Requirement: 三种运行矩阵 RSS 下降至少 5%

候选发布物 SHALL 在相同设备、节点与流量下使 Xray AsIs、Xray IPIfNonMatch 和 sing-box 原生路由的五次稳定 RSS 中位数分别不高于各自基线的 95%。

#### Scenario: 三种运行矩阵 RSS 下降至少 5%

- **GIVEN** Xray AsIs、Xray IPIfNonMatch、sing-box 原生路由分别完成相同冷启动、稳定等待和固定流量预热
- **WHEN** 每个基线/候选组合采集五次同口径 RSS 并计算中位数
- **THEN** 每种矩阵的候选中位数均不超过自身基线的 95%，报告记录绝对值与百分比

## MODIFIED Requirements

### Requirement: 十三个托管组与残差引用（原十六个组，删除 GFW/BilibiliHMT/独立 tld-not-cn）

默认 `passwall2-groups.yaml` SHALL 按 `reject → youtube → netflix → spotify → tiktok → ai → apple_services → telegram → google → proxy → private → cn → direct` 定义 13 个组，其中 Google 同时引用 GeoSite/GeoIP `[google]`，`proxy` 只引用 GeoSite `[proxy]`，`direct` 只引用 GeoSite `[direct]`，其他保留组引用不变。

#### Scenario: 默认组 ID、顺序与标签精确匹配

- **GIVEN** 仓库默认 groups 配置
- **WHEN** 加载并渲染 PassWall2 fragment
- **THEN** 组 ID 与上述 13 项顺序完全相同，不含 `gfw`、`tld_not_cn`、`bilibili_hmt`，`crs_google` 同时引用 `geosite:google` 与 `geoip:google`，且 `crs_proxy` 的 domain list 只引用 `geosite:proxy`

### Requirement: 十三个 Shadowrocket 策略组与合并代理出口（原十六个独立 select 组）

Shadowrocket conf SHALL 为 13 个托管组生成具名策略组；最终 `proxy` 中已实体合并的 GFW/proxy/tld-not-cn 规则统一使用 `proxy` remarks，不生成独立 GFW 或“非中国域名”策略组，其余组候选与默认出口继续遵循旧契约。

#### Scenario: tld-not-cn 规则归入代理规则

- **GIVEN** `proxy` 组只引用已实体合并的最终 `proxy`，remarks 为「代理规则」
- **WHEN** 渲染 `[Proxy Group]` 与 `[Rule]`
- **THEN** 只生成一个「代理规则」策略组，最终 `proxy` 展开的规则都引用该 remarks，配置中不存在 GFW 或“非中国域名”策略组

## REMOVED Requirements

### Requirement: 独立 GFW 托管组

移除原因：GFW 规则并入最终 `proxy`，独立组会重复装载并保留无意义的单独出口；最终 DAT 同时删除 `gfw` 标签。

### Requirement: BilibiliHMT 来源、标签与策略组

移除原因：用户要求完整移除，不保留远程来源、双侧 DAT 标签、PassWall2/Shadowrocket 组、custom 模板或文档兼容入口。

### Requirement: tld-not-cn 与 proxy 可独立选择出口

移除原因：proxy 残差会把可证明由 `tld-not-cn` 覆盖的规则交给后者，二者分配不同出口将破坏优化前后的路由等价。

### Requirement: 独立 tld_not_cn 托管与策略组

移除原因：`tld-not-cn` 规则实体已并入最终 `proxy`；继续保留独立组只会形成重复策略容器和节点配置双真相。来源与 custom 模板继续作为中间输入，但最终 DAT 删除 `tld-not-cn` 标签。

## 方案设计

### 架构与组件

- **`internal/geodat`**：从 GeoSite protobuf wire 格式无损解码 tag、domain kind、value 与 attributes；当前编译器产生的 bool true attribute 还原为 `@key`，遇到 false、int 或未知 typed value 则失败。该层提供给残差推导和 Shadowrocket 投影复用；Shadowrocket 可继续跳过 regexp/attributes，但共享解码层不得丢失它们。
- **`internal/geosite` 残差推导器**：接受已展开 tag 集合、组优先级与生成规格，返回确定排序的 `model.DomainRule` 列表和计数报告；写入器先校验目标与待删除标签，再原子覆写 `proxy`/`direct` 并删除中间标签。
- **`internal/app` 编译编排**：GeoSite 第一次把基础数据编译到 staging 的 intermediate 路径；先生成并原位覆写 `proxy`，删除 `gfw`、`tld-not-cn`，再以最终 proxy 语义推导并原位覆写 `direct`；第二次才把最终 `geosite.dat` 写入 publish。GeoIP 保持单次编译，但输出使用 manifest required GeoIP 作为 `wantedList`，不把完整底包标签带入 publish。
- **`internal/passwall` 迁移模板**：在删除并重建 managed `shunt_rules` 前扫描所有 Shunt 节点，完成历史三组字段的全局预检后才暂存节点改动；备份/恢复范围扩展到节点 options。
- **`internal/shadowrocket` 策略关系**：按最终 13 组直接渲染；`proxy` 组只展开最终同名标签，不保留 tld 专用 alias 分支。
- **目标与 manifest 契约**：最终 GeoSite 要求 `proxy`、`direct`，禁止 `gfw`、`tld-not-cn`、两个旧 `crs-*` 标签与 BilibiliHMT identifiers；目标注册允许基础标签接受正常输入；最终 GeoIP 的输出闭集直接取 manifest required GeoIP。

### 数据流

```text
load/fetch/custom → merge 基础 data-merged → compile intermediate geosite.dat
  → lossless decode → derive proxy → merge tld-not-cn → overwrite proxy
  → delete gfw/tld-not-cn → derive + overwrite direct
  → compile final geosite.dat → project manifest-required tags + compile geoip.dat
  → validate groups + required/forbidden tags
  → render PassWall2 installer + Shadowrocket conf
  → verify 八资产 → commit publish
```

`proxy` 残差先以 `tld-not-cn` 作为等价覆盖推导，再把完整 `tld-not-cn` 规则实体精确去重并入最终 `proxy`；`direct` 残差使用已经原位更新的最终活动组顺序推导。中间 DAT 不位于正式 publish，不创建或复用 `FinalLookup`。

安装器数据流：下载与 SHA 校验 → UCI 全量预检 → 备份原节点/规则/URL → 暂存迁移及新 fragment → 运行规则更新与校验 → 一次提交；任一步失败按备份恢复并输出结果。

### 关键接口

```go
type SiteTag struct {
    Name    string
    Domains []model.DomainRule // suffix/full/keyword/regexp + bool-true Attr keys
}

type ResidualSpec struct {
    OutputTag           string
    BasisTags           []string
    EarlierActiveTags   []string
    LaterEquivalentTags []string
}

type ResidualReport struct {
    Basis, Kept, Removed int
}

func DecodeGeoSite(data []byte) (map[string]SiteTag, error)
func Derive(tags map[string]SiteTag, spec ResidualSpec) ([]model.DomainRule, ResidualReport, error)
```

接口表达职责边界，不要求实施沿用具体文件名；但无损解码、纯函数推导、明确生成规格和可审计计数是契约。Shadowrocket 直接使用每个现行组的 remarks；proxy 只展开最终同名标签，也不给 YAML 增加平台专属字段。

### 覆盖证明规则

- full 候选：相同 full、标签边界正确的相同/父级 suffix、或值中确定包含的已有 keyword 可覆盖
- suffix 候选：标签边界正确的相同/父级 suffix，或值中确定包含的已有 keyword 可覆盖
- keyword 候选：已有 keyword 是候选 keyword 的子串时可覆盖
- regexp 候选：仅 regexp 文本与 attributes 均相同可覆盖
- attributes 排序去重成规范化集合，只有集合完全相同才做覆盖推断；大小写/IDN 以编译后 DAT 的规范值为准
- 不组合多条规则证明覆盖，不做 regexp 语言包含，不借助 GeoIP

### 错误处理

- 中间 DAT 解码不认识 wire kind/attributes：失败，不降级到 Shadowrocket 的有损解码
- basis tag 缺失、残差为空、原位目标/删除目标无效或移除规则无证明：失败并输出 tag/规则上下文；校验失败不得提前覆写原文件
- 最终 DAT 缺 `proxy`/`direct`，或仍含中间标签、旧 `crs-*` 标签或 BilibiliHMT：失败，事务 Abort
- GeoIP 输出白名单为空或含非法 tag：在编译前失败；最终缺少任一 required GeoIP：探针失败并事务 Abort
- 任一 Shunt 节点冲突：完成全部预检后统一失败，live UCI 零修改
- 安装器提交后校验失败：恢复节点 options、managed rules 与 global URLs，并明确输出 rollback 成功/失败

## 测试与验收策略

| Scenario / 检查项 | 维度 | 执行方式 | 验收证据 |
|-------------------|------|---------|---------|
| include 与属性在残差输入中已展开 | integration | 任务内 TDD | fixture DAT 的 kind/value/attributes 完整 |
| skip-compile 不生成残差发布物 | integration | 任务内 TDD | build stage 断言 |
| GFW 与 proxy 合并且服务组优先 | unit | 任务内 TDD | residual golden |
| 不借用后序直连组裁剪 proxy | unit | 任务内 TDD | 候选保留断言 |
| tld-not-cn 与 cn 使 direct 规则不可达 | unit | 任务内 TDD | residual golden |
| GeoIP 不作为域名覆盖证据 | unit | 任务内 TDD | 无 GeoIP 输入接口且规则保留 |
| suffix 标签边界不产生假覆盖 | unit | 任务内 TDD | `ample.com` 反例 |
| 正则只做同类等值去重 | unit | 任务内 TDD | regexp/attrs 参数化测试 |
| 输出顺序可重现 | unit | 任务内 TDD | 两次字节相同 |
| 最终 GeoSite 编译输出可重现 | unit/integration | 验收修复 TDD | 乱序 fixture 与同源重复构建逐字节相同 |
| 中间基础标签缺失 | unit/integration | 任务内 TDD | 缺失 tag 错误与 publish 未切换 |
| 未知 GeoSite 字段不被规范化吞掉 | unit/integration | 验收修复 TDD | document/tag/domain 未知字段拒绝且旧 publish 不变 |
| 中间 DAT 不污染最终探测缓存 | integration | 任务内 TDD | final lookup 新鲜读取 |
| 无法回写的 typed attribute 失败 | unit/integration | 任务内 TDD | false/int/unknown attribute 拒绝测试 |
| 原位残差替换完整标签 | integration | 任务内 TDD | proxy/direct 存在且四个禁止标签缺失 |
| reject 指纹不受优化影响 | integration | 任务内 TDD | reject 无损指纹一致 |
| 完整底包被投影为五个活动标签 | unit/integration | quick-fix TDD | 生成配置 `wantedList` + 最终 DAT 精确 tag 集合 |
| Google IP 进入双端规则 | unit/integration | quick-fix TDD | groups golden + PassWall2/Shadowrocket 规则断言 |
| proxy 缺失时继承 gfw | integration-shell | 任务内 TDD | fake UCI 三族迁移 |
| 单侧关闭按非空一侧归一化 | integration-shell | 任务内 TDD | 明确启用全集并输出警告 |
| gfw 与 proxy 冲突时零修改失败 | integration-shell | 任务内 TDD | UCI 前后快照一致 |
| 重复安装保持幂等 | integration-shell | 任务内 TDD | 第二次 diff 为空 |
| 三个旧组均继承 Default | integration-shell | 任务内 TDD | `_default` 迁入唯一 proxy |
| 等价出口条件不成立 | integration-shell | 任务内 TDD | 节点/字段错误且零修改 |
| 规范 proxy-only 配置重复安装 | integration-shell | 任务内 TDD | 第二次配置内容不变 |
| 后续步骤失败时恢复节点 | integration-shell | 任务内 TDD | rules/nodes/URLs 全量回读 |
| 成功安装后无废弃字段 | integration-shell | 任务内 TDD | fake/live UCI 均无旧字段 |
| 默认代理和三组直连不被重写 | integration-shell | 任务内 TDD | 相关节点字段前后快照一致 |
| proxy custom 先合并再裁剪 | unit/integration | 任务内 TDD | repository/custom contract 与最终规则断言 |
| 最终 DAT 不含 BilibiliHMT | integration | 任务内 TDD | 双侧 forbidden probe + `rg` 契约 |
| 默认组 ID、顺序与标签精确匹配 | unit | 任务内 TDD | config/render golden |
| tld-not-cn 规则归入代理规则 | unit/integration | 任务内 TDD | 单一 Proxy Group、最终标签与规则 policy golden |
| Xray AsIs 域名路由等价 | e2e-router | 验收任务 (D) | 基线/候选命中与出口对照 |
| Xray IPIfNonMatch 二次 IP 匹配等价 | e2e-router | 验收任务 (D) | 两轮匹配日志与出口对照 |
| sing-box 原生路由等价 | e2e-router | 验收任务 (D) | 基线/候选命中与出口对照 |
| 代理规则减少至少 30% 且 Google IP 增长受控 | perf-static | 验收任务 (D) | GFW/proxy/tld 同源计数、Shadowrocket GeoSite/Google IP 分项与最终 DAT 字节报告 |
| 三种运行矩阵 RSS 下降至少 5% | perf-router | 验收任务 (D) | 每格 5 次样本、中位数与百分比 |
| 仓库完整回归 | regression | 验收任务 (D) | `go test ./...`、构建、releasecmp 全绿 |
| Shadowrocket 真机导入 | e2e-device | 验收任务 (D) | DEFERRED，记录于验收报告 |

## 风险与边缘情况

- 最终 `geosite.dat` 不再同时存储完整公共标签与残差副本；相同输入下其字节数必须低于兼容方案基线，否则静态性能验收失败。
- 独立 `tld_not_cn` 组与 `proxy` 的独立选择能力被主动移除；PassWall2 用预检拒绝旧自定义分歧，Shadowrocket 只保留合并后的 proxy 组。
- 上游规则会漂移，因此 GFW/proxy/tld-not-cn 的 30%、Shadowrocket 总字节 105% 上限与 GeoSite 派生部分严格下降门槛都使用同一输入的相对基线，不使用历史 Release 的固定行数。
- `geoip:google` 当前会给 Shadowrocket 增加大量内联 CIDR；已批准接受受控增长，但必须独立统计该规则块，超过同输入基线 5% 或 Google IP 之外出现净增长都视为回归。
- RSS 包含核心基线内存和设备噪声；固定设备/配置/流量、冷启动、稳定等待、五次中位数用于降低方差，任一矩阵未达 5% 即不通过性能验收。
- 所有 Shunt 节点都会被预检；一个未使用但仍保存冲突字段的节点也会阻止安装，以避免静默损坏未来切换配置。
- 旧节点若只启用 gfw/proxy 其中一组且 tld-not-cn 与归一结果等价，合并后会按非空一侧启用三者全集；这是用户批准的迁移规则，安装器必须显式告警，路由等价验收不覆盖该非兼容配置。
- `gfw`、`tld-not-cn`、`proxy`、`direct` 继续允许 custom 扩展并参与中间编译，但最终只发布原位残差 `proxy`、`direct`；直接依赖已删除标签的外部消费者会破坏，这是用户批准的显式不兼容变更。
- BilibiliHMT 从双侧 DAT 删除可能影响直接消费该标签的外部用户，这是用户批准的显式破坏性变化。
- 最终 GeoIP 删除未引用国家与特殊标签，直接消费这些标签的外部配置会失效；这是用户批准的不兼容精简，恢复标签必须先加入 manifest 所需来源或托管组。
- BilibiliHMT 旧规则、旧策略组或旧节点独立出口不参加路由等价比较；候选中它们必须不存在，而不是保持旧出口。

## 开放问题

无。实施细节必须在上述接口边界和量化门槛内完成，不得通过放宽证明或验收阈值解决失败。
