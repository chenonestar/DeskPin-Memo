import type { PetAccessory, PetGlasses } from '../types'
import type { HolidayAccessory, IdleAct, PetMood } from '../fun'

/** 一次性的小特效：摸头冒爱心、完成事项叼骨头、升级撒花。 */
export type PetEffect = 'hearts' | 'bone' | 'confetti' | null

/**
 * 正在做的动作：二期互动（握手、打喷嚏、揉眼睛、推眼镜、叼着球、守着水碗），
 * 三期自主动作（巡逻由外层平移实现，这里不画）与翻肚皮。
 */
export type PetAction = 'shake' | 'sneeze' | 'rub' | 'push' | 'carry' | 'water' | 'belly' | Exclude<IdleAct, 'patrol'> | null

interface Props {
  mood: PetMood
  accessory: PetAccessory | HolidayAccessory
  glasses: PetGlasses
  effect: PetEffect
  action?: PetAction
  /** 眼珠朝向（-1..1），用于跟着鼠标看 */
  look?: { x: number; y: number }
  size?: number
}

const INK = '#2B2A24'
const FUR = '#E39A4F'
const CREAM = '#FFF1DC'
const EAR = '#9A5B2E'

/** 戴眼镜的狗狗。纯矢量手绘（无外部素材），表情 / 装扮 / 特效全部由参数决定，动画在 styles.css。 */
export function Dog({ mood, accessory, glasses, effect, action = null, look, size = 64 }: Props) {
  const sneeze = action === 'sneeze'
  const rub = action === 'rub'
  const yawn = action === 'yawn'
  const clean = action === 'clean' // 擦眼镜：眼镜拿在爪子里
  const read = action === 'read'
  const lie = action === 'liedown'
  const asleep = ((mood === 'sleeping' || mood === 'dozing') && !rub) || yawn
  const joyful = mood === 'happy' || mood === 'celebrate' || action === 'belly'
  const worried = mood === 'worried'
  const lx = look ? Math.max(-1, Math.min(1, look.x)) * 3 : 0
  const ly = read ? 3 : look ? Math.max(-1, Math.min(1, look.y)) * 2.5 : 0
  const scarfColor = accessory === 'redscarf' ? '#D93025' : '#4A7BD0'
  const scarfTail = accessory === 'redscarf' ? '#B3261E' : '#3D68B4'
  return (
    <svg viewBox="0 0 200 200" width={size} height={size} className={`dog mood-${mood}${action ? ' act-' + action : ''}`} data-mood={mood} data-action={action ?? ''} aria-hidden="true">
      <g className="dog-all">
        {/* 尾巴 */}
        <path className="dog-tail" d="M142 156 C164 152 172 132 166 120" fill="none" stroke={FUR} strokeWidth="12" strokeLinecap="round" />
        {/* 身体 */}
        <ellipse cx="100" cy="156" rx="44" ry="36" fill={FUR} />
        <ellipse cx="100" cy="164" rx="26" ry="24" fill={CREAM} />
        <ellipse cx="81" cy="188" rx="13" ry="8" fill={CREAM} stroke="#C97C35" strokeWidth="2" />
        {action === 'shake'
          ? <ellipse cx="146" cy="138" rx="13" ry="8" fill={CREAM} stroke="#C97C35" strokeWidth="2" transform="rotate(-35 146 138)" data-paw="raised" />
          : <ellipse cx="119" cy="188" rx="13" ry="8" fill={CREAM} stroke="#C97C35" strokeWidth="2" />}
        {action === 'scratch' && (
          <g className="dog-scratchleg" data-pose="scratch">
            <ellipse cx="60" cy="140" rx="10" ry="26" fill={FUR} transform="rotate(32 60 140)" />
            <ellipse cx="44" cy="112" rx="11" ry="7" fill={CREAM} stroke="#C97C35" strokeWidth="2" />
          </g>
        )}
        {(accessory === 'scarf' || accessory === 'redscarf') && (
          <g data-acc={accessory}>
            <path d="M62 120 Q100 138 138 120 L136 132 Q100 150 64 132 Z" fill={scarfColor} />
            <path d="M118 132 L126 160 L114 158 L108 136 Z" fill={scarfTail} />
          </g>
        )}
        {/* 头 */}
        <g className="dog-head">
          {accessory === 'bunny' && (
            <g data-acc="bunny">
              <ellipse cx="78" cy="22" rx="10" ry="26" fill="#fff" stroke="#E6D9D2" strokeWidth="2" transform="rotate(-12 78 22)" />
              <ellipse cx="78" cy="24" rx="5" ry="18" fill="#F7C6CF" transform="rotate(-12 78 24)" />
              <ellipse cx="122" cy="22" rx="10" ry="26" fill="#fff" stroke="#E6D9D2" strokeWidth="2" transform="rotate(12 122 22)" />
              <ellipse cx="122" cy="24" rx="5" ry="18" fill="#F7C6CF" transform="rotate(12 122 24)" />
            </g>
          )}
          <ellipse cx="100" cy="76" rx="50" ry="46" fill={FUR} />
          <path className="dog-ear-l" d="M58 46 C38 46 30 72 34 98 C36 112 50 114 56 102 C62 88 64 64 70 52 Z" fill={EAR} />
          <path className="dog-ear-r" d="M142 46 C162 46 170 72 166 98 C164 112 150 114 144 102 C138 88 136 64 130 52 Z" fill={EAR} />
          <path d="M100 34 C94 46 92 60 96 70 L104 70 C108 60 106 46 100 34 Z" fill={CREAM} />
          {worried ? (
            <g stroke={CREAM} strokeWidth="4" strokeLinecap="round">
              <path d="M72 58 L86 52" /><path d="M128 58 L114 52" />
            </g>
          ) : (
            <g fill={CREAM}><ellipse cx="80" cy="56" rx="5" ry="3" /><ellipse cx="120" cy="56" rx="5" ry="3" /></g>
          )}
          {/* 眼睛 */}
          {sneeze ? (
            <g fill="none" stroke={INK} strokeWidth="3.5" strokeLinecap="round" data-eyes="squeeze">
              <path d="M73 74 L86 79 L73 84" /><path d="M127 74 L114 79 L127 84" />
            </g>
          ) : rub ? (
            <g fill="none" stroke={INK} strokeWidth="3.5" strokeLinecap="round" data-eyes="rub">
              <path d="M73 80 Q80 85 87 80" /><circle cx="120" cy="79" r="5" fill={INK} stroke="none" />
            </g>
          ) : lie ? (
            <g fill="none" stroke={INK} strokeWidth="3.5" strokeLinecap="round" data-eyes="half">
              <path d="M73 79 L87 79" /><path d="M113 79 L127 79" />
            </g>
          ) : clean ? (
            <g fill={INK} data-eyes="bare"><circle cx="80" cy="79" r="3.5" /><circle cx="120" cy="79" r="3.5" /></g>
          ) : asleep ? (
            <g fill="none" stroke={INK} strokeWidth="3.5" strokeLinecap="round" data-eyes="closed">
              <path d="M73 80 Q80 85 87 80" /><path d="M113 80 Q120 85 127 80" />
            </g>
          ) : joyful ? (
            <g fill="none" stroke={INK} strokeWidth="3.5" strokeLinecap="round" data-eyes="happy">
              <path d="M73 81 Q80 73 87 81" /><path d="M113 81 Q120 73 127 81" />
            </g>
          ) : (
            <g className="dog-eyes" data-eyes="open">
              <g className="dog-pupils" transform={`translate(${lx} ${ly})`} data-look={`${lx.toFixed(1)},${ly.toFixed(1)}`}>
                <circle cx="80" cy="78" r={worried ? 5 : 6} fill={INK} />
                <circle cx="120" cy="78" r={worried ? 5 : 6} fill={INK} />
                <circle cx="82" cy="76" r="2" fill="#fff" />
                <circle cx="122" cy="76" r="2" fill="#fff" />
              </g>
            </g>
          )}
          <ellipse cx="100" cy="104" rx="26" ry="19" fill={CREAM} />
          <ellipse cx="66" cy="100" rx={sneeze ? 10 : 7} ry={sneeze ? 6 : 4.5} fill={sneeze ? '#F27F86' : '#F7A9A0'} opacity={joyful || sneeze ? 0.95 : 0.7} />
          <ellipse cx="134" cy="100" rx={sneeze ? 10 : 7} ry={sneeze ? 6 : 4.5} fill={sneeze ? '#F27F86' : '#F7A9A0'} opacity={joyful || sneeze ? 0.95 : 0.7} />
          {/* 眼镜：担心时滑到鼻尖；打喷嚏时被震歪 */}
          {!clean && (
            <g className="dog-glasses" transform={sneeze ? 'rotate(-14 100 78) translate(0 4)' : worried ? 'translate(0 6)' : undefined} data-glasses={glasses}>
              <Glasses kind={glasses} />
            </g>
          )}
          <ellipse cx="100" cy="96" rx="9" ry="6.5" fill={INK} />
          <ellipse cx="97" cy="94" rx="3" ry="1.6" fill="#fff" opacity="0.6" />
          {yawn ? (
            <ellipse cx="100" cy="114" rx="9" ry="10" fill="#7A2E2E" stroke={INK} strokeWidth="2.5" data-mouth="yawn" />
          ) : worried ? (
            <path d="M88 113 Q94 108 100 113 Q106 118 112 113" fill="none" stroke={INK} strokeWidth="2.5" strokeLinecap="round" />
          ) : (
            <path d="M100 102 L100 108 M100 108 Q92 115 86 109 M100 108 Q108 115 114 109" fill="none" stroke={INK} strokeWidth="2.5" strokeLinecap="round" />
          )}
          {sneeze && (
            <g stroke="#9FB7D9" strokeWidth="3" strokeLinecap="round" data-fx="sneeze">
              <path d="M112 100 L126 96" /><path d="M112 104 L128 106" /><path d="M110 108 L122 116" />
            </g>
          )}
          {rub && <ellipse cx="74" cy="90" rx="11" ry="8" fill={CREAM} stroke="#C97C35" strokeWidth="2" data-paw="rub" />}
          {action === 'carry' && (
            <g data-carry="ball"><circle cx="100" cy="118" r="11" fill="#F5C542" stroke={INK} strokeWidth="2" /><path d="M90 114 Q100 122 110 114" fill="none" stroke="#E5484D" strokeWidth="2.5" /></g>
          )}
          {clean && <circle className="dog-puff" cx="122" cy="118" r="9" fill="#DCEBF7" opacity="0.8" data-fx="puff" />}
          {!asleep && !worried && !sneeze && !lie && action !== 'carry' && (
            <g className="dog-tongue">
              <path d="M94 111 Q94 124 100 124 Q106 124 106 111 Z" fill="#F07C7C" stroke={INK} strokeWidth="2" />
              <path d="M100 113 L100 120" stroke="#C85A5A" strokeWidth="1.5" strokeLinecap="round" />
            </g>
          )}
          {accessory === 'partyhat' && (
            <g data-acc="partyhat">
              <path d="M76 40 L100 2 L124 40 Z" fill="#9B6CD8" />
              <path d="M84 28 L116 28 M90 18 L110 18" stroke="#F5C542" strokeWidth="4" />
              <circle cx="100" cy="4" r="6" fill="#F05A7E" />
            </g>
          )}
          {accessory === 'santa' && (
            <g data-acc="santa">
              <path d="M70 40 Q96 0 136 26 L130 38 Z" fill="#D93025" />
              <rect x="66" y="35" width="70" height="10" rx="5" fill="#fff" />
              <circle cx="136" cy="26" r="7" fill="#fff" />
            </g>
          )}
          {accessory === 'mugwort' && (
            <g data-acc="mugwort" fill="#4E9A51">
              <path d="M98 34 Q90 10 74 4 Q86 16 92 34 Z" /><path d="M100 34 Q104 8 118 0 Q110 16 104 34 Z" /><path d="M99 34 Q96 18 100 6 Q102 18 101 34 Z" fill="#6DB66F" />
            </g>
          )}
          {accessory === 'hat' && (
            <g data-acc="hat">
              <path d="M70 40 Q100 4 130 40 Z" fill="#E5484D" />
              <rect x="66" y="36" width="68" height="9" rx="4.5" fill="#fff" />
              <circle cx="100" cy="10" r="7" fill="#fff" />
            </g>
          )}
          {accessory === 'crown' && (
            <path data-acc="crown" d="M72 40 L76 16 L88 30 L100 10 L112 30 L124 16 L128 40 Z" fill="#F5C542" stroke="#C99A1B" strokeWidth="3" strokeLinejoin="round" />
          )}
        </g>
        {accessory === 'bow' && (
          <g data-acc="bow">
            <path d="M100 128 L82 118 L82 140 Z" fill="#E5484D" />
            <path d="M100 128 L118 118 L118 140 Z" fill="#E5484D" />
            <circle cx="100" cy="129" r="5" fill="#C23A3E" />
          </g>
        )}
      </g>
      {read && (
        <g data-prop="book">
          <path d="M66 150 L100 158 L100 186 L66 178 Z" fill="#fff" stroke={INK} strokeWidth="2" />
          <path d="M134 150 L100 158 L100 186 L134 178 Z" fill="#F4F1E8" stroke={INK} strokeWidth="2" />
          <path className="dog-page" d="M100 158 L130 152 L130 178 L100 184 Z" fill="#fff" stroke={INK} strokeWidth="1.5" />
          <path d="M72 160 L94 165 M72 166 L94 171 M106 165 L126 160 M106 171 L126 166" stroke="#B9B4A6" strokeWidth="2" />
        </g>
      )}
      {clean && (
        <g className="dog-heldglasses" transform="translate(52 150) scale(0.6)" data-prop="glasses-in-paw"><Glasses kind={glasses} /></g>
      )}
      {action === 'shakeoff' && (
        <g stroke="#B9946A" strokeWidth="3" strokeLinecap="round" data-fx="shakeoff">
          <path d="M28 120 L16 116" /><path d="M26 136 L12 138" /><path d="M172 120 L184 116" /><path d="M174 136 L188 138" />
        </g>
      )}
      {action === 'chase' && (
        <g className="dog-stars" fill="#F5C542" data-fx="dizzy">
          <path d="M70 18 l3 7 7 1 -5 5 1 7 -6 -3 -6 3 1 -7 -5 -5 7 -1z" /><path d="M120 10 l3 7 7 1 -5 5 1 7 -6 -3 -6 3 1 -7 -5 -5 7 -1z" />
        </g>
      )}
      {action === 'butterfly' && (
        <g className="dog-butterfly" data-prop="butterfly">
          <ellipse cx="0" cy="0" rx="8" ry="5" fill="#F28CB8" transform="rotate(-30)" /><ellipse cx="0" cy="0" rx="8" ry="5" fill="#9B6CD8" transform="rotate(30)" />
          <rect x="-1.5" y="-6" width="3" height="12" rx="1.5" fill={INK} />
        </g>
      )}
      {action === 'water' && (
        <g data-prop="bowl">
          <path d="M150 178 L196 178 L190 196 L156 196 Z" fill="#4A7BD0" stroke={INK} strokeWidth="2" />
          <ellipse cx="173" cy="178" rx="23" ry="5" fill="#9FD3F5" stroke={INK} strokeWidth="2" />
        </g>
      )}
      {asleep && !yawn && <text className="dog-zzz" x="150" y="36" fontSize="26" fontWeight="700" fill="#6C7A96">Z<tspan fontSize="18" dy="-10">z</tspan></text>}
      {effect === 'hearts' && (
        <g className="dog-fx" data-fx="hearts" fill="#F05A7E">
          <path d="M160 40 c-6-10-20-4-14 6 l14 14 14-14 c6-10-8-16-14-6z" />
          <path d="M34 54 c-4-7-14-3-10 4 l10 10 10-10 c4-7-6-11-10-4z" />
        </g>
      )}
      {effect === 'bone' && (
        <g className="dog-fx" data-fx="bone" transform="translate(70 120) rotate(-12)">
          <rect x="8" y="6" width="44" height="10" rx="5" fill="#FFF8EC" stroke={INK} strokeWidth="2" />
          <circle cx="8" cy="5" r="6" fill="#FFF8EC" stroke={INK} strokeWidth="2" /><circle cx="8" cy="17" r="6" fill="#FFF8EC" stroke={INK} strokeWidth="2" />
          <circle cx="52" cy="5" r="6" fill="#FFF8EC" stroke={INK} strokeWidth="2" /><circle cx="52" cy="17" r="6" fill="#FFF8EC" stroke={INK} strokeWidth="2" />
        </g>
      )}
      {effect === 'confetti' && (
        <g className="dog-fx dog-confetti" data-fx="confetti">
          {[['#E5484D', 30, 20], ['#F5C542', 60, 6], ['#4A7BD0', 140, 12], ['#2E8B57', 170, 30], ['#F05A7E', 100, 2], ['#9B6CD8', 20, 50]].map(([c, x, y], i) => (
            <rect key={i} x={x as number} y={y as number} width="8" height="12" rx="2" fill={c as string} transform={`rotate(${i * 37} ${x} ${y})`} />
          ))}
        </g>
      )}
    </svg>
  )
}

function Glasses({ kind }: { kind: PetGlasses }) {
  const frame = kind === 'gold' ? '#C99A1B' : INK
  const lens = kind === 'sunglasses' ? 'rgba(30,30,40,0.85)' : 'rgba(255,255,255,0.12)'
  const legs = <path d="M65 75 L54 70 M135 75 L146 70" stroke={frame} strokeWidth="4" strokeLinecap="round" />
  const bridge = <path d="M95 77 Q100 72 105 77" fill="none" stroke={frame} strokeWidth="4" strokeLinecap="round" />
  const shine = (
    <g stroke="#fff" strokeWidth="3" strokeLinecap="round" opacity="0.85">
      <path d="M71 71 L76 67" /><path d="M111 71 L116 67" />
    </g>
  )
  return (
    <>
      {kind === 'square' || kind === 'sunglasses' ? (
        <g fill={lens} stroke={frame} strokeWidth="4">
          <rect x="65" y="65" width="30" height="25" rx="6" /><rect x="105" y="65" width="30" height="25" rx="6" />
        </g>
      ) : (
        <g fill={lens} stroke={frame} strokeWidth="4">
          <circle cx="80" cy="78" r="15" /><circle cx="120" cy="78" r="15" />
        </g>
      )}
      {bridge}{legs}{shine}
    </>
  )
}
