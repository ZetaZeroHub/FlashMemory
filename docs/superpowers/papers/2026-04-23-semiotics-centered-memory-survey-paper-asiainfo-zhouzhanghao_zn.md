# 迈向演化型 AI 系统的符号记忆架构

> arXiv 风格综述-立场预印本草稿的中文一比一审阅稿  
> 作者：周章浩，技术总监，亚信科技（AsiaInfo）  
> 日期：2026-04-23  
> 本稿综合整理了 [2026-04-22-memory-01-core-thesis.md](./2026-04-22-memory-01-core-thesis.md) 至 [2026-04-22-memory-08-experiments-metrics-roadmap.md](./2026-04-22-memory-08-experiments-metrics-roadmap.md) 的八篇记忆研究分稿。

## 摘要

大语言模型智能体在检索、规划与工具使用方面已经取得了快速进展，但当前多数记忆架构仍然把记忆视为一个存储与检索问题，而不是一个意义形成、语义漂移与集体稳定化问题。本文将一个更大研究计划中的八章内容整合为一篇以符号学为中心的 arXiv 风格综述-立场稿。本文的核心主张是，演化型 AI 记忆应被建模为一种符号秩序，而不是文本缓存：记忆项不仅仅是一段内容切片，而是 `sign`、`object` 与 `interpretant` 之间动态维持的关系。围绕这一主张，本文以认知心理学来刻画内部层级，以博弈论来解释竞争与更新，以社会传播理论来解释扩散与制度化，并以计算语言学来操作化语义漂移。由此得到的框架覆盖五个层次：从 semiotic memory unit 与认知记忆层，到 self-play harness、semiosphere 式集体记忆、传播感知审计，以及作为对象锚定与时间溯源底座的 FlashMemory。本文进一步主张，未来的 AI 记忆研究应从单纯最大化 recall，转向优化 semiotic stability、interpretive divergence management、symbol grounding strength、canon quality 与 repair efficiency。本文并非只是总结这八章内容，而是将其重组为一套适合预印本的研究问题、体系结构与评测议程，用于指导演化型记忆系统的构建与检验。

## 关键词

符号记忆，AI 智能体，记忆架构，semiosphere，self-play harness，FlashMemory，语义漂移，集体记忆

> [图 1 占位]
> Teaser 图：从 `sign-object-interpretant` 到 `individual memory -> self-play harness -> semiosphere -> FlashMemory substrate` 的全景总览。
> 使用 [2026-04-23-semiotics-centered-memory-survey-paper-asiainfo-zhouzhanghao-author-notes.md](./2026-04-23-semiotics-centered-memory-survey-paper-asiainfo-zhouzhanghao-author-notes.md) 中的 `FIG-1`。

## 1. 引言

当前 AI 记忆的主流范式仍然过于狭窄。在大多数现有系统中，记忆被视为一种辅助缓冲区，用于存放观察、摘要或向量化切片，以备后续检索。这一范式带来了有价值的工程收益，但它无法解释为什么长时程智能体经常出现语义漂移、口号化摘要、不稳定规范，以及跨智能体与跨模态的解释冲突。更深层的瓶颈并不只是系统会遗忘，而是系统无法维持自己所记住内容的可操作意义。

本文主张，应当把 AI 记忆重新表述为一个符号学问题。一个有用的记忆并不仅仅是一段被存储的文本，而是一个由符号、该符号所指向的对象，以及使这层关系具有可行动性的解释项所共同构成的结构化关系。一旦以这种方式理解记忆，那些在标准检索系统中原本处于边缘位置的现象就会变成核心研究问题：符号如何在多轮摘要中发生漂移，竞争性解释如何被选择或淘汰，局部经验如何被提升为群体规范，以及多个智能体如何在身处部分不同符号体制时仍然维持对象层的一致性 [5], [6]。

这种重构也会重新安排相邻学科的角色。在本文构建的框架中，符号学是概念中心；认知心理学解释记忆为何需要内部层级 [1]–[4]；博弈论解释为何记忆更新本质上是竞争性的而非中性的；社会传播理论解释为何重复暴露与簇状强化会把局部主张转化为共享惯例 [7]；计算语言学则为语义漂移、别名膨胀与解释分歧提供可测量的指标 [8]。这些学科共同把记忆从被动仓库转化为一种不断演化的意义秩序。

### 1.1 贡献

本文的主要贡献如下：

1. 它将 AI 记忆重新界定为一个围绕 `sign-object-interpretant` 稳定性的符号学问题，而非扩展版检索缓冲区问题。
2. 它把一个更大研究计划中的八章内容重组为一套连贯架构，覆盖 semiotic memory unit、认知层、自博弈演化、集体 semiosphere 与传播感知审计。
3. 它将 FlashMemory 定位为面向记忆演化的对象感知研究底座，而不是单体化的终端智能体平台。
4. 它提出了一套以 semiotic stability、interpretive divergence、symbol grounding strength、canon quality 与 repair efficiency 为中心的评测议程。

### 1.2 论文组织

本文其余部分遵循标准预印本叙事结构。第 2 节回顾最相关的研究脉络，并澄清促使我们进行符号学重构的概念缺口。第 3 节形式化本文的问题定义，并推出演化型记忆系统的设计需求。第 4 节把八章内容综合为一套统一的符号记忆架构。第 5 节讨论 FlashMemory 作为使该架构可经验研究的底座。第 6 节提出评测议程与研究路线图。第 7 节讨论局限性与开放问题，第 8 节作结。

## 2. 预备知识与相关工作

### 2.1 智能体系统中的记忆

近期关于智能体记忆的研究已经表明，记忆不只是短期检索，但这一领域仍然缺少一个关于“意义稳定性”的统一解释。Generative Agents 提出了一种架构，在该架构中，智能体存储经历、检索相关轨迹，并综合出更高层反思来塑造未来行为 [9]。Reflexion 则表明，基于语言的自反馈可以通过情景式记忆缓冲充当一种轻量级学习回路 [10]。CAMEL 展示了多智能体角色扮演如何暴露出涌现式协作模式与差异化沟通机制 [11]。MemGPT 借鉴操作系统思想提出了层级记忆方案，通过分层分页管理超长上下文 [12]。这些工作共同证明，记忆、反思与交互都很重要。然而，它们仍然倾向于把记忆主要理解为已存储文本或上下文管理问题，而不是一种符号结构。

### 2.2 认知心理学与内部层级

经典认知心理学为内部记忆组织提供了更有原则的解释。Atkinson 与 Shiffrin 的控制过程模型分解了记忆存储与路由功能 [1]。Baddeley 与 Hitch 进一步说明，工作记忆不是被动缓冲，而是进行操作与加工的主动工作空间 [2]。Tulving 对情景记忆与语义记忆的区分在这里尤其关键：系统应当区分“发生过什么”和“通常成立什么” [3]。Nelson 与 Narens 又引入了元层视角，使系统可以监控自己的不确定性、新鲜度与可靠性 [4]。这些传统共同表明，一个可行的智能体记忆架构必须支持分层存储、受控巩固与显式自监控。

### 2.3 符号学与集体意义

符号学提供了当前智能体记忆文献所缺少的概念中心。Peirce 的 `sign-object-interpretant` 三元关系明确指出，意义不能被还原为符号与所指对象的二元对应；解释本身并不是次级产物，而是构成性的部分 [5]。Lotman 的 semiosphere 则把这一洞见从个体解释扩展到集体符号组织，强调边界、翻译、核心-边缘结构以及异质共存 [6], [14]。这对 AI 记忆尤其关键，因为多智能体系统并不共享同一种同质语言。coding agent、reviewer、architect、文档、图示与人类操作者，分别栖居在部分不同的符号区域之中。许多所谓的“记忆失败”，因此更适合被理解为翻译、对齐或制度化失败，而不是简单的存储失败。

### 2.4 社会传播与计算语言学

另外两个领域使这种符号学重构具备可操作性。社会传播理论解释了重复且簇状的传播如何把局部主张转化为共享惯例。Centola 的工作尤其重要，因为它显示出，行为采纳往往依赖于网络结构中的强化，而不是一次成功暴露即可完成 [7]。计算语言学则为研究意义如何随时间变化提供了互补工具。Hamilton 等人的工作说明，可以通过历时性表示变化来定量研究语义漂移 [8]。综合起来，这些研究表明，当前记忆缓冲区真正的后继者不应只是更大的上下文窗口，而应是一种能够跟踪符号如何在时间与智能体之间移动、压缩与漂移的传播感知型架构。

## 3. 问题定义与设计需求

本文所综合的八章研究稿以一个比现有多数记忆工作更强的问题定义为起点。核心问题不是如何存储更多观察，而是如何在反复摘要、翻译、争议与制度化复用中维持可操作意义。一个记忆系统失败，并不只是因为某个符号还在却“记错了”，而是因为该符号不再指向同一个对象，不再支持兼容的解释项，或已经悄然脱离其原始溯源链条。在长时程智能体系统中，这类失败会不断累积，最终表现为脆弱规划、口号化规范与集体失配。

这一问题定义推出五项设计需求。第一，**对象锚定** 必须显式存在：每条耐久主张都应绑定到可检查的对象上，如代码实体、文档、事故或决策。第二，**解释多元性** 必须可表示：竞争性解释应作为一级对象共存，而不是被压扁成一段被覆盖的摘要。第三，**时间溯源** 必须保留，使系统能够追踪某条主张何时、何地、为何进入记忆。第四，**分阶段巩固** 必须把瞬时观察与稳定知识、制度化 canon 区分开来。第五，**可争议性与可修复性** 必须内建于系统中，使记忆质量通过实际使用、质疑与修订来判定，而不是依赖写入时启发式。

这些需求也澄清了本文的边界。本文提出的框架并不是关于人类意义的一般理论，也不是说每个智能体管线都必须显式实现所有符号学概念。它更准确地说，是一个用于工程化记忆系统的研究计划，目标是在系统不断演化的过程中，依然保持可解释、可审计与可更新。其实践目标是构建能够穿越长时间尺度、异质角色与反复转述而不失去对象层 grounding 的记忆架构。

## 4. 一种以符号学为中心的记忆架构

### 4.1 记忆作为符号秩序

底层研究计划的第一章提出了一个核心论断：记忆应被视为不断演化的符号秩序，而不是信息存储器。这一转向把设计目标从最大化 recall 改写为在时间、任务与沟通语境中维持可操作意义。一旦意义稳定性成为核心，记忆架构就必须显式建模主张如何被生产、流通、争议与稳定化。对象锚定、溯源与受控晋升不再是可有可无的实现细节，而成为记忆设计的构成性部分。

这一论断的实践后果是，记忆不能再被建模为扁平缓存。耐久记忆必须为歧义、竞争性解释与修订历史保留空间。一个只存储转述文本的系统会逐渐积累自指性的摘要与不稳定口号。相反，一个存储 `sign-object-interpretant` 关系的系统则能够区分词汇层变化与真正的语义变化。这一命题为后续所有架构组件提供了理论铰链。

### 4.2 Semiotic Memory Unit

第二章给出了该架构的最小研究对象：**semiotic memory unit（SMU）**。SMU 不是通用切片、笔记或消息，而是一种 claim-level 结构，至少包含七个字段：`sign`、`object`、`interpretant`、`context`、`provenance`、`confidence` 与 `social_status`。这一设计把 Peirce 的三元关系操作化，并补入工程与实验所必需的最小控制面。`sign` 捕获表层表达，`object` 把主张锚定到可检查实体，`interpretant` 记录可行动理解，`context` 刻画使用条件，`provenance` 保证可审计性，`confidence` 校准信任强度，而 `social_status` 则标记该主张当前处于私人线索、局部假设、争议解释、暂时共识还是制度 canon。

SMU 抽象的关键优势在于，它把对象身份与解释多元性分离开来。竞争性解释不再被揉成一段模糊摘要，而是作为不同单元通过 `supports`、`contests`、`supersedes` 等关系显式连接。这使解释本身成为可操纵的研究对象，而不是 prompt 措辞的偶然副产物。它也为整个架构提供了一个可被索引、晋升、质疑、弃用并随时间测量的操作原子。

> [图 2 占位]
> SMU schema 图：以中心节点 `Semiotic Memory Unit` 向外连接 `sign / object / interpretant / context / provenance / confidence / social_status / relations / metrics / timestamps`。
> 使用 [2026-04-23-semiotics-centered-memory-survey-paper-asiainfo-zhouzhanghao-author-notes.md](./2026-04-23-semiotics-centered-memory-survey-paper-asiainfo-zhouzhanghao-author-notes.md) 中的 `FIG-2`。

### 4.3 认知层与记忆生理结构

第三章借用认知心理学来规定 SMU 在智能体内部应如何被处理。这里提出的是一种五层“记忆生理结构”：知觉缓冲、工作记忆、情景记忆、语义记忆与元记忆。之所以需要分层，是因为同一个单元不应同时承担所有角色。原始观察属于短暂缓冲；任务相关解释属于工作记忆；事件轨迹属于情景记忆；稳定化后的主张属于语义记忆；而新鲜度、不确定性、冲突与检索诊断则属于元记忆 [1]–[4]。

这种分层视角可以防止两类常见病理。第一类是上下文蔓延，即所有观察都被当作同等耐久，导致系统嘈杂且脆弱。第二类是过早 canon 化，即单次事件被过早一般化，进而变成过度自信的规范。通过区分 SMU 处于哪里、如何迁移，这一架构把记忆管理转化为关于路由、巩固与遗忘的原则性理论。

### 4.4 Self-Play Harness 作为演化引擎

第四章主张，记忆质量不能仅在写入时被判定。一条主张只有在被检索、被执行、被挑战，并最终被强化或修复之后，才真正证明自己。因此，`self-play harness` 被重新定义为一种记忆演化环境，而不是跑 benchmark 的工具。它负责生成任务、分配异质角色、组织反思或辩论、评估下游结果，并据此更新记忆状态。在这一设定中，自博弈不只是 agent-versus-agent 竞争，而是一种系统性暴露不稳定解释、过度自信抽象与脆弱惯例的机制 [10], [11], [13], [15]。

从符号学视角看，harness 是解释项竞争的场所；从博弈论视角看，它是主张在反复使用中获得或失去策略价值的场所。harness 因而补上了符号记忆理论所必需的选择机制。没有它，记忆就只是档案；有了它，记忆才真正成为演化系统，因为系统可以奖励 grounding 更强、可复用性更高的解释，并压低那些误导性强或锚定薄弱的主张。

### 4.5 Semiosphere、传播与集体记忆

第五章与第六章把问题从个体智能体扩展到集体符号生态。沿着 Lotman 的思路，该架构把多智能体或人机协作环境视为由多个部分重叠的符号区域构成的 **semiosphere** [6], [14]。代码审查话语、事故话语、产品话语、架构话语以及人类 tacit 话语并不是可以随意互换的通道；它们是具有不同翻译成本与不同失真风险的结构化区域。跨区域翻译既是创新来源，也是错误来源。

传播进一步使问题复杂化。记忆不会只是“保存下来”，它会被反复转述、压缩、翻译与再分发。在这个过程中，符号可能逐渐偏离对象，别名可能不断膨胀，局部解释也可能固化为误导性口号。计算语言学通过漂移信号、anchor retention 与 alias growth 使这一过程变得可测 [8]。社会传播理论则解释了为何某些主张只有在重复且簇状强化之后才会变成规范 [7]。因此，集体记忆不能被简化为许多个体记忆的并行堆叠；它是一片处于运动中的结构化符号场。

> [图 3 占位]
> 集体记忆与 semiosphere 图：多个语义区域（`Code`、`Design`、`Incident`、`Product`、`Human Tacit`）围绕共享 object graph 分布，通过 boundary filters 与 translation edges 相连；右侧展示 propagation、compression、drift 与 stabilization。
> 使用 [2026-04-23-semiotics-centered-memory-survey-paper-asiainfo-zhouzhanghao-author-notes.md](./2026-04-23-semiotics-centered-memory-survey-paper-asiainfo-zhouzhanghao-author-notes.md) 中的 `FIG-3`。

### 4.6 FlashMemory 作为底座层

第七章把整个架构落回 FlashMemory。与其把 FlashMemory 定位成一个完整的智能体产品，这一章更倾向于把它解释为负责对象锚定、多模态符号索引、时间溯源、图关系与研究可追踪性的底座。这是一种策略性收缩。符号记忆架构首先需要的是稳定的对象层，而不是又一个单体编排栈。FlashMemory 之所以有希望，恰恰在于它已经靠近那些可检查对象：代码实体、文档、图关系与跨制品链接。

这种底座视角也澄清了分工。FlashMemory 应提供对象图管理、多模态符号索引、时间溯源，以及服务于工作记忆的检索/装配原语。harness 应消费这些原语来组织试验、辩论、冲突解决与策略更新。智能体则作为承担角色的解释者栖居在系统之中，而不是成为记忆本身的所有者。这样的分离使整个研究计划更加模块化、可审计，也更适合科学研究。

## 5. FlashMemory 作为符号记忆研究底座

上面的架构推出了一个关于基础设施的策略性判断。如果符号记忆要求对象锚定、溯源、时间可追踪性与多模态符号对齐，那么在评价任何端到端记忆产品之前，底座就必须先为这些功能而优化。FlashMemory 的特殊价值在于，它允许研究者把符号主张绑定到可检查制品之上。这使得我们可以重放记忆演化、检查争议主张、追踪 canon 化事件，并比较不同时期与不同传播机制下的演化轨迹，而不只能依赖零散的 agent transcript。

这种底座化 framing 在方法论上也很关键。当前相当一部分智能体记忆研究难以复现，是因为记忆被隐藏在 prompt、不可解释摘要或临时 JSON blob 之中。相比之下，以 FlashMemory 为中心的底座可以把对象锚点、时间谱系、关系图与晋升历史暴露为可检查的研究对象。从这个意义上说，底座不只是工程基础设施，它本身就是把符号记忆从诱人隐喻转化为可证伪系统研究所需的方法论组成部分。

## 6. 评测议程与研究路线图

如果这套八章研究计划要真正成为经验研究议程，那么评测故事就必须像架构故事一样清楚。第一组指标关注 **semiotic stability**：一条主张在被摘要、翻译、辩论或跨智能体传播之后，是否仍然指向同一个对象，并维持兼容的解释项。第二组指标关注 **interpretive divergence**：当多个智能体面对同一对象时，它们的解释项有多大分歧，这种分歧是显式的还是潜伏的。第三组指标是 **symbol grounding strength**：一条主张中的符号是否仍然锚定在可检查对象上，还是已经漂移成悬浮口号。

下一组指标则面向规范形成与修复。**Canon quality** 用来衡量那些被晋升为共享规范的主张，是否真的具备可复用性、精确性与低风险。**Repair efficiency** 用来衡量当一个薄弱解释已经建立之后，系统能多快恢复。之所以需要这组指标，是因为长寿命记忆系统中代价最高的失败，往往不是孤立的检索失误，而是那些被反复且自信复用的糟糕规范。因此，评估演化型记忆不仅需要个体正确性指标，也需要制度层指标。

从实验设计角度看，这一框架自然导出一个对比矩阵。第一维改变记忆表示：plain chunks、object-anchored chunks 或完整 SMUs。第二维改变内部层级：flat memory 与 layered memory。第三维改变演化机制：无反思、单智能体反思、辩论或完整 self-play harness。第四维改变社会结构：单智能体、人机对、多智能体簇或多区域 semiosphere。这样的设计使整个研究计划保持可证伪性。如果符号记忆没有价值，它应当在受控对比下失败；如果它有价值，它的优势也应体现为稳定性、grounding 与修复能力上的可测提升，而不只是“对话看起来更聪明”。

> [图 4 占位]
> 评测路线图：一个四维实验矩阵（`Memory Unit / Memory Layers / Evolution Mechanism / Social Structure`）连接到五类核心指标（`Semiotic Stability`、`Interpretive Divergence`、`Symbol Grounding Strength`、`Canon Quality`、`Repair Efficiency`）。
> 使用 [2026-04-23-semiotics-centered-memory-survey-paper-asiainfo-zhouzhanghao-author-notes.md](./2026-04-23-semiotics-centered-memory-survey-paper-asiainfo-zhouzhanghao-author-notes.md) 中的 `FIG-4`。

## 7. 讨论、局限性与开放问题

本文最强的主张，也最容易被误解。把符号学置于中心，并不是把 AI 系统还原成一种人文学隐喻，而是指出许多高级记忆失败在根本上都是 `sign-object-interpretant` 对齐失败。单靠认知心理学，无法解释集体规范如何形成；单靠博弈论，无法解释两个智能体为何看似分歧却其实指向了不同对象；单靠社会传播理论，无法解释为何反复传播的主张有时会固化为空口号；单靠计算语言学，可以检测漂移，却不能独自规定什么才算稳定意义。符号学提供了一个整合层，使这些学科不再只是松散相邻，而是彼此约束。

这一提案也有清晰的局限。第一，符号化表示会增加 schema 与标注成本。第二，对象锚定在软件仓库中相对自然，但在开放世界领域中更难成立，因为“可检查对象”本身没有那么清晰。第三，`social_status` 的状态迁移需要治理规则，既要避免过早 canon 化，也要避免过度保守。第四，当前领域仍缺少专门针对争议意义与跨 semiosphere 翻译而设计的大规模 benchmark。这些并不是枝节性的实现问题，而是这项研究真正的科学工作量所在。

由此也直接引出若干开放问题。在不同 false canonization 成本的领域中，晋升阈值应如何变化？什么样的 harness 冲突最能诊断薄弱解释项？semiosphere 边界应如何被学习出来，而不是人工硬编码？哪些表示最能揭示“词汇变化”与“对象层漂移”的区别？这些问题表明，本文提出的架构并不是某次设计工作的终点，而是一项可测量研究计划的起点。

## 8. 结论

本文把一个更大研究包中的八章内容重组为一篇单独的 arXiv 风格综述-立场稿。核心结论是，演化型 AI 记忆应当围绕符号结构来构建，而不是被理解为扩展版上下文管理。由此得到的架构以 semiotic memory units 为起点，经由认知记忆层与 self-play harness 向上扩展，进入集体 semiosphere 与传播感知审计，并由 FlashMemory 这一对象感知底座提供支撑。认知心理学解释内部层级，博弈论解释竞争与更新，社会传播理论解释规范形成，计算语言学解释如何使漂移可测。但符号学仍然是中心，因为只有它真正说明了：长时程挑战的关键，不是保存更多内容，而是保存意义。

如果这一综合判断是正确的，那么下一代 AI 记忆研究将不再只问如何存得更多、概括得更好、检索得更快，而会转向追问：如何在智能体、制品、时间尺度与制度之间维持并治理意义。这是一个更难的问题，但也是最值得回答的问题。

## 参考文献

[1] R. C. Atkinson and R. M. Shiffrin, “Human Memory: A Proposed System and its Control Processes,” in *Psychology of Learning and Motivation*, 1968. [链接](https://doi.org/10.1016/S0079-7421(08)60422-3)

[2] A. D. Baddeley and G. J. Hitch, “Working Memory,” in *Psychology of Learning and Motivation*, 1974.

[3] E. Tulving, “Episodic and Semantic Memory,” in *Organization of Memory*, 1972. [链接](https://cir.nii.ac.jp/crid/1574231874408386176?lang=en)

[4] T. O. Nelson and L. Narens, “Metamemory: A Theoretical Framework and New Findings,” in *Psychology of Learning and Motivation*, 1990. [链接](https://doi.org/10.1016/S0079-7421(08)60053-5)

[5] T. L. Short, “Peirce’s Theory of Signs,” *Stanford Encyclopedia of Philosophy*, 2021 archive. [链接](https://plato.stanford.edu/archives/sum2021/entries/peirce-semiotics/)

[6] J. Lotman and W. Clark, “On the Semiosphere,” *Sign Systems Studies*, vol. 33, no. 1, pp. 205–229, 2005. [链接](https://doi.org/10.12697/SSS.2005.33.1.09)

[7] D. Centola, “The Spread of Behavior in an Online Social Network Experiment,” *Science*, vol. 329, no. 5996, pp. 1194–1197, 2010. [链接](https://doi.org/10.1126/science.1185231)

[8] W. L. Hamilton, J. Leskovec, and D. Jurafsky, “Diachronic Word Embeddings Reveal Statistical Laws of Semantic Change,” in *ACL 2016*, pp. 1489–1501, 2016. [链接](https://aclanthology.org/P16-1141/)

[9] J. S. Park, J. C. O’Brien, C. J. Cai, M. R. Morris, P. Liang, and M. S. Bernstein, “Generative Agents: Interactive Simulacra of Human Behavior,” in *UIST 2023*, 2023. [链接](https://arxiv.org/abs/2304.03442)

[10] N. Shinn, F. Cassano, A. Gopinath, K. Narasimhan, and S. Yao, “Reflexion: Language Agents with Verbal Reinforcement Learning,” in *NeurIPS 2023*, 2023. [链接](https://proceedings.neurips.cc/paper_files/paper/2023/hash/1b44b878bb782e6954cd888628510e90-Abstract-Conference.html)

[11] G. Li, H. A. A. K. Hammoud, H. Itani, D. Khizbullin, and B. Ghanem, “CAMEL: Communicative Agents for ‘Mind’ Exploration of Large Language Model Society,” *NeurIPS 2023*. [链接](https://arxiv.org/abs/2303.17760)

[12] C. Packer, S. Wooders, K. Lin, V. Fang, S. G. Patil, I. Stoica, and J. E. Gonzalez, “MemGPT: Towards LLMs as Operating Systems,” 2024. [链接](https://arxiv.org/abs/2310.08560)

[13] A. Madaan et al., “Self-Refine: Iterative Refinement with Self-Feedback,” 2023. [链接](https://doi.org/10.48550/arXiv.2303.17651)

[14] W. Nöth, “The Topography of Yuri Lotman’s Semiosphere,” *International Journal of Cultural Studies*, vol. 18, no. 1, pp. 11–26, 2015. [链接](https://doi.org/10.1177/1367877914528114)

[15] S. Yao et al., “ReAct: Synergizing Reasoning and Acting in Language Models,” *ICLR 2023*. [链接](https://arxiv.org/abs/2210.03629)

> 作者写作笔记、插图提示词与内部自检保存在 [2026-04-23-semiotics-centered-memory-survey-paper-asiainfo-zhouzhanghao-author-notes.md](./2026-04-23-semiotics-centered-memory-survey-paper-asiainfo-zhouzhanghao-author-notes.md)。
