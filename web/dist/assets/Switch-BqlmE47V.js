import{E as e,ct as t,ft as n,g as r,k as i}from"./echarts-Cw2yHLaZ.js";import{A as a,Bt as o,D as s,Ft as c,Lt as l,Pt as u,Rt as d,St as f,T as p,_t as m,bt as h,dt as g,ft as _,kt as v,ut as y,x as b,y as x,zt as S}from"./auth-CYMAsBzV.js";import{O as C,j as w}from"./vue-core-BDTvi3xZ.js";import{s as T}from"./get-CsENiSKu.js";import{i as E}from"./index-ChsYpqrV.js";function D(e){let{primaryColor:t,opacityDisabled:n,borderRadius:r,textColor3:i}=e;return Object.assign(Object.assign({},E),{iconColor:i,textColor:`white`,loadingColor:t,opacityDisabled:n,railColor:`rgba(0, 0, 0, .14)`,railColorActive:t,buttonBoxShadow:`0 1px 4px 0 rgba(0, 0, 0, 0.3), inset 0 0 1px 0 rgba(0, 0, 0, 0.05)`,buttonColor:`#FFF`,railBorderRadiusSmall:r,railBorderRadiusMedium:r,railBorderRadiusLarge:r,buttonBorderRadiusSmall:r,buttonBorderRadiusMedium:r,buttonBorderRadiusLarge:r,boxShadowFocus:`0 0 0 2px ${v(t,{alpha:.2})}`})}var O={name:`Switch`,common:x,self:D},k=c(`switch`,`
 height: var(--n-height);
 min-width: var(--n-width);
 vertical-align: middle;
 user-select: none;
 -webkit-user-select: none;
 display: inline-flex;
 outline: none;
 justify-content: center;
 align-items: center;
`,[l(`children-placeholder`,`
 height: var(--n-rail-height);
 display: flex;
 flex-direction: column;
 overflow: hidden;
 pointer-events: none;
 visibility: hidden;
 `),l(`rail-placeholder`,`
 display: flex;
 flex-wrap: none;
 `),l(`button-placeholder`,`
 width: calc(1.75 * var(--n-rail-height));
 height: var(--n-rail-height);
 `),c(`base-loading`,`
 position: absolute;
 top: 50%;
 left: 50%;
 transform: translateX(-50%) translateY(-50%);
 font-size: calc(var(--n-button-width) - 4px);
 color: var(--n-loading-color);
 transition: color .3s var(--n-bezier);
 `,[p({left:`50%`,top:`50%`,originalTransform:`translateX(-50%) translateY(-50%)`})]),l(`checked, unchecked`,`
 transition: color .3s var(--n-bezier);
 color: var(--n-text-color);
 box-sizing: border-box;
 position: absolute;
 white-space: nowrap;
 top: 0;
 bottom: 0;
 display: flex;
 align-items: center;
 line-height: 1;
 `),l(`checked`,`
 right: 0;
 padding-right: calc(1.25 * var(--n-rail-height) - var(--n-offset));
 `),l(`unchecked`,`
 left: 0;
 justify-content: flex-end;
 padding-left: calc(1.25 * var(--n-rail-height) - var(--n-offset));
 `),u(`&:focus`,[l(`rail`,`
 box-shadow: var(--n-box-shadow-focus);
 `)]),d(`round`,[l(`rail`,`border-radius: calc(var(--n-rail-height) / 2);`,[l(`button`,`border-radius: calc(var(--n-button-height) / 2);`)])]),S(`disabled`,[S(`icon`,[d(`rubber-band`,[d(`pressed`,[l(`rail`,[l(`button`,`max-width: var(--n-button-width-pressed);`)])]),l(`rail`,[u(`&:active`,[l(`button`,`max-width: var(--n-button-width-pressed);`)])]),d(`active`,[d(`pressed`,[l(`rail`,[l(`button`,`left: calc(100% - var(--n-offset) - var(--n-button-width-pressed));`)])]),l(`rail`,[u(`&:active`,[l(`button`,`left: calc(100% - var(--n-offset) - var(--n-button-width-pressed));`)])])])])])]),d(`active`,[l(`rail`,[l(`button`,`left: calc(100% - var(--n-button-width) - var(--n-offset))`)])]),l(`rail`,`
 overflow: hidden;
 height: var(--n-rail-height);
 min-width: var(--n-rail-width);
 border-radius: var(--n-rail-border-radius);
 cursor: pointer;
 position: relative;
 transition:
 opacity .3s var(--n-bezier),
 background .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier);
 background-color: var(--n-rail-color);
 `,[l(`button-icon`,`
 color: var(--n-icon-color);
 transition: color .3s var(--n-bezier);
 font-size: calc(var(--n-button-height) - 4px);
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 display: flex;
 justify-content: center;
 align-items: center;
 line-height: 1;
 `,[p()]),l(`button`,`
 align-items: center; 
 top: var(--n-offset);
 left: var(--n-offset);
 height: var(--n-button-height);
 width: var(--n-button-width-pressed);
 max-width: var(--n-button-width);
 border-radius: var(--n-button-border-radius);
 background-color: var(--n-button-color);
 box-shadow: var(--n-button-box-shadow);
 box-sizing: border-box;
 cursor: inherit;
 content: "";
 position: absolute;
 transition:
 background-color .3s var(--n-bezier),
 left .3s var(--n-bezier),
 opacity .3s var(--n-bezier),
 max-width .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier);
 `)]),d(`active`,[l(`rail`,`background-color: var(--n-rail-color-active);`)]),d(`loading`,[l(`rail`,`
 cursor: wait;
 `)]),d(`disabled`,[l(`rail`,`
 cursor: not-allowed;
 opacity: .5;
 `)])]),A=Object.assign(Object.assign({},a.props),{size:String,value:{type:[String,Number,Boolean],default:void 0},loading:Boolean,defaultValue:{type:[String,Number,Boolean],default:!1},disabled:{type:Boolean,default:void 0},round:{type:Boolean,default:!0},"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],checkedValue:{type:[String,Number,Boolean],default:!0},uncheckedValue:{type:[String,Number,Boolean],default:!1},railStyle:Function,rubberBand:{type:Boolean,default:!0},spinProps:Object,onChange:[Function,Array]}),j,M=e({name:`Switch`,props:A,slots:Object,setup(e){j===void 0&&(j=typeof CSS<`u`?CSS.supports===void 0?!1:CSS.supports(`width`,`max(1px)`):!0);let{mergedClsPrefixRef:i,inlineThemeDisabled:s,mergedComponentPropsRef:c}=_(e),l=a(`Switch`,`-switch`,k,O,e,i),u=y(e,{mergedSize(t){return e.size===void 0?t?t.mergedSize.value:c?.value?.Switch?.size||`medium`:e.size}}),{mergedSizeRef:d,mergedDisabledRef:p}=u,m=t(e.defaultValue),h=T(n(e,`value`),m),v=r(()=>h.value===e.checkedValue),b=t(!1),x=t(!1),S=r(()=>{let{railStyle:t}=e;if(t)return t({focused:x.value,checked:v.value})});function E(t){let{"onUpdate:value":n,onChange:r,onUpdateValue:i}=e,{nTriggerFormInput:a,nTriggerFormChange:o}=u;n&&f(n,t),i&&f(i,t),r&&f(r,t),m.value=t,a(),o()}function D(){let{nTriggerFormFocus:e}=u;e()}function A(){let{nTriggerFormBlur:e}=u;e()}function M(){e.loading||p.value||(h.value===e.checkedValue?E(e.uncheckedValue):E(e.checkedValue))}function N(){x.value=!0,D()}function P(){x.value=!1,A(),b.value=!1}function F(t){e.loading||p.value||t.key===` `&&(h.value===e.checkedValue?E(e.uncheckedValue):E(e.checkedValue),b.value=!1)}function I(t){e.loading||p.value||t.key===` `&&(t.preventDefault(),b.value=!0)}let L=r(()=>{let{value:e}=d,{self:{opacityDisabled:t,railColor:n,railColorActive:r,buttonBoxShadow:i,buttonColor:a,boxShadowFocus:s,loadingColor:c,textColor:u,iconColor:f,[o(`buttonHeight`,e)]:p,[o(`buttonWidth`,e)]:m,[o(`buttonWidthPressed`,e)]:h,[o(`railHeight`,e)]:g,[o(`railWidth`,e)]:_,[o(`railBorderRadius`,e)]:v,[o(`buttonBorderRadius`,e)]:y},common:{cubicBezierEaseInOut:b}}=l.value,x,S,T;return j?(x=`calc((${g} - ${p}) / 2)`,S=`max(${g}, ${p})`,T=`max(${_}, calc(${_} + ${p} - ${g}))`):(x=w((C(g)-C(p))/2),S=w(Math.max(C(g),C(p))),T=C(g)>C(p)?_:w(C(_)+C(p)-C(g))),{"--n-bezier":b,"--n-button-border-radius":y,"--n-button-box-shadow":i,"--n-button-color":a,"--n-button-width":m,"--n-button-width-pressed":h,"--n-button-height":p,"--n-height":S,"--n-offset":x,"--n-opacity-disabled":t,"--n-rail-border-radius":v,"--n-rail-color":n,"--n-rail-color-active":r,"--n-rail-height":g,"--n-rail-width":_,"--n-width":T,"--n-box-shadow-focus":s,"--n-loading-color":c,"--n-text-color":u,"--n-icon-color":f}}),R=s?g(`switch`,r(()=>d.value[0]),L,e):void 0;return{handleClick:M,handleBlur:P,handleFocus:N,handleKeyup:F,handleKeydown:I,mergedRailStyle:S,pressed:b,mergedClsPrefix:i,mergedValue:h,checked:v,mergedDisabled:p,cssVars:s?void 0:L,themeClass:R?.themeClass,onRender:R?.onRender}},render(){let{mergedClsPrefix:e,mergedDisabled:t,checked:n,mergedRailStyle:r,onRender:a,$slots:o}=this;a?.();let{checked:c,unchecked:l,icon:u,"checked-icon":d,"unchecked-icon":f}=o,p=!(m(u)&&m(d)&&m(f));return i(`div`,{role:`switch`,"aria-checked":n,class:[`${e}-switch`,this.themeClass,p&&`${e}-switch--icon`,n&&`${e}-switch--active`,t&&`${e}-switch--disabled`,this.round&&`${e}-switch--round`,this.loading&&`${e}-switch--loading`,this.pressed&&`${e}-switch--pressed`,this.rubberBand&&`${e}-switch--rubber-band`],tabindex:this.mergedDisabled?void 0:0,style:this.cssVars,onClick:this.handleClick,onFocus:this.handleFocus,onBlur:this.handleBlur,onKeyup:this.handleKeyup,onKeydown:this.handleKeydown},i(`div`,{class:`${e}-switch__rail`,"aria-hidden":`true`,style:r},h(c,t=>h(l,n=>t||n?i(`div`,{"aria-hidden":!0,class:`${e}-switch__children-placeholder`},i(`div`,{class:`${e}-switch__rail-placeholder`},i(`div`,{class:`${e}-switch__button-placeholder`}),t),i(`div`,{class:`${e}-switch__rail-placeholder`},i(`div`,{class:`${e}-switch__button-placeholder`}),n)):null)),i(`div`,{class:`${e}-switch__button`},h(u,t=>h(d,n=>h(f,r=>i(s,null,{default:()=>this.loading?i(b,Object.assign({key:`loading`,clsPrefix:e,strokeWidth:20},this.spinProps)):this.checked&&(n||t)?i(`div`,{class:`${e}-switch__button-icon`,key:n?`checked-icon`:`icon`},n||t):!this.checked&&(r||t)?i(`div`,{class:`${e}-switch__button-icon`,key:r?`unchecked-icon`:`icon`},r||t):null})))),h(c,t=>t&&i(`div`,{key:`checked`,class:`${e}-switch__checked`},t)),h(l,t=>t&&i(`div`,{key:`unchecked`,class:`${e}-switch__unchecked`},t)))))}});export{M as t};