import{E as e,ct as t,ft as n,g as r,k as i}from"./echarts-Cw2yHLaZ.js";import{B as a,E as o,F as s,M as c,R as l,b as u,w as d}from"./auth-0ZchciGS.js";import{O as f,j as p}from"./vue-core-BDTvi3xZ.js";import{B as m,G as h,J as g,K as _,N as v,P as y,X as b,Y as x,Z as S,i as C,t as w}from"./light-BluJl8SD.js";import{c as T}from"./_plugin-vue_export-helper-BkHhRWuE.js";import{i as E}from"./index-wbJthrLm.js";function D(e){let{primaryColor:t,opacityDisabled:n,borderRadius:r,textColor3:i}=e;return Object.assign(Object.assign({},E),{iconColor:i,textColor:`white`,loadingColor:t,opacityDisabled:n,railColor:`rgba(0, 0, 0, .14)`,railColorActive:t,buttonBoxShadow:`0 1px 4px 0 rgba(0, 0, 0, 0.3), inset 0 0 1px 0 rgba(0, 0, 0, 0.05)`,buttonColor:`#FFF`,railBorderRadiusSmall:r,railBorderRadiusMedium:r,railBorderRadiusLarge:r,buttonBorderRadiusSmall:r,buttonBorderRadiusMedium:r,buttonBorderRadiusLarge:r,boxShadowFocus:`0 0 0 2px ${m(t,{alpha:.2})}`})}var O={name:`Switch`,common:w,self:D},k=_(`switch`,`
 height: var(--n-height);
 min-width: var(--n-width);
 vertical-align: middle;
 user-select: none;
 -webkit-user-select: none;
 display: inline-flex;
 outline: none;
 justify-content: center;
 align-items: center;
`,[g(`children-placeholder`,`
 height: var(--n-rail-height);
 display: flex;
 flex-direction: column;
 overflow: hidden;
 pointer-events: none;
 visibility: hidden;
 `),g(`rail-placeholder`,`
 display: flex;
 flex-wrap: none;
 `),g(`button-placeholder`,`
 width: calc(1.75 * var(--n-rail-height));
 height: var(--n-rail-height);
 `),_(`base-loading`,`
 position: absolute;
 top: 50%;
 left: 50%;
 transform: translateX(-50%) translateY(-50%);
 font-size: calc(var(--n-button-width) - 4px);
 color: var(--n-loading-color);
 transition: color .3s var(--n-bezier);
 `,[d({left:`50%`,top:`50%`,originalTransform:`translateX(-50%) translateY(-50%)`})]),g(`checked, unchecked`,`
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
 `),g(`checked`,`
 right: 0;
 padding-right: calc(1.25 * var(--n-rail-height) - var(--n-offset));
 `),g(`unchecked`,`
 left: 0;
 justify-content: flex-end;
 padding-left: calc(1.25 * var(--n-rail-height) - var(--n-offset));
 `),h(`&:focus`,[g(`rail`,`
 box-shadow: var(--n-box-shadow-focus);
 `)]),x(`round`,[g(`rail`,`border-radius: calc(var(--n-rail-height) / 2);`,[g(`button`,`border-radius: calc(var(--n-button-height) / 2);`)])]),b(`disabled`,[b(`icon`,[x(`rubber-band`,[x(`pressed`,[g(`rail`,[g(`button`,`max-width: var(--n-button-width-pressed);`)])]),g(`rail`,[h(`&:active`,[g(`button`,`max-width: var(--n-button-width-pressed);`)])]),x(`active`,[x(`pressed`,[g(`rail`,[g(`button`,`left: calc(100% - var(--n-offset) - var(--n-button-width-pressed));`)])]),g(`rail`,[h(`&:active`,[g(`button`,`left: calc(100% - var(--n-offset) - var(--n-button-width-pressed));`)])])])])])]),x(`active`,[g(`rail`,[g(`button`,`left: calc(100% - var(--n-button-width) - var(--n-offset))`)])]),g(`rail`,`
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
 `,[g(`button-icon`,`
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
 `,[d()]),g(`button`,`
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
 `)]),x(`active`,[g(`rail`,`background-color: var(--n-rail-color-active);`)]),x(`loading`,[g(`rail`,`
 cursor: wait;
 `)]),x(`disabled`,[g(`rail`,`
 cursor: not-allowed;
 opacity: .5;
 `)])]),A=Object.assign(Object.assign({},C.props),{size:String,value:{type:[String,Number,Boolean],default:void 0},loading:Boolean,defaultValue:{type:[String,Number,Boolean],default:!1},disabled:{type:Boolean,default:void 0},round:{type:Boolean,default:!0},"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],checkedValue:{type:[String,Number,Boolean],default:!0},uncheckedValue:{type:[String,Number,Boolean],default:!1},railStyle:Function,rubberBand:{type:Boolean,default:!0},spinProps:Object,onChange:[Function,Array]}),j,M=e({name:`Switch`,props:A,slots:Object,setup(e){j===void 0&&(j=typeof CSS<`u`?CSS.supports===void 0?!1:CSS.supports(`width`,`max(1px)`):!0);let{mergedClsPrefixRef:i,inlineThemeDisabled:o,mergedComponentPropsRef:s}=y(e),l=C(`Switch`,`-switch`,k,O,e,i),u=c(e,{mergedSize(t){return e.size===void 0?t?t.mergedSize.value:s?.value?.Switch?.size||`medium`:e.size}}),{mergedSizeRef:d,mergedDisabledRef:m}=u,h=t(e.defaultValue),g=T(n(e,`value`),h),_=r(()=>g.value===e.checkedValue),b=t(!1),x=t(!1),w=r(()=>{let{railStyle:t}=e;if(t)return t({focused:x.value,checked:_.value})});function E(t){let{"onUpdate:value":n,onChange:r,onUpdateValue:i}=e,{nTriggerFormInput:o,nTriggerFormChange:s}=u;n&&a(n,t),i&&a(i,t),r&&a(r,t),h.value=t,o(),s()}function D(){let{nTriggerFormFocus:e}=u;e()}function A(){let{nTriggerFormBlur:e}=u;e()}function M(){e.loading||m.value||(g.value===e.checkedValue?E(e.uncheckedValue):E(e.checkedValue))}function N(){x.value=!0,D()}function P(){x.value=!1,A(),b.value=!1}function F(t){e.loading||m.value||t.key===` `&&(g.value===e.checkedValue?E(e.uncheckedValue):E(e.checkedValue),b.value=!1)}function I(t){e.loading||m.value||t.key===` `&&(t.preventDefault(),b.value=!0)}let L=r(()=>{let{value:e}=d,{self:{opacityDisabled:t,railColor:n,railColorActive:r,buttonBoxShadow:i,buttonColor:a,boxShadowFocus:o,loadingColor:s,textColor:c,iconColor:u,[S(`buttonHeight`,e)]:m,[S(`buttonWidth`,e)]:h,[S(`buttonWidthPressed`,e)]:g,[S(`railHeight`,e)]:_,[S(`railWidth`,e)]:v,[S(`railBorderRadius`,e)]:y,[S(`buttonBorderRadius`,e)]:b},common:{cubicBezierEaseInOut:x}}=l.value,C,w,T;return j?(C=`calc((${_} - ${m}) / 2)`,w=`max(${_}, ${m})`,T=`max(${v}, calc(${v} + ${m} - ${_}))`):(C=p((f(_)-f(m))/2),w=p(Math.max(f(_),f(m))),T=f(_)>f(m)?v:p(f(v)+f(m)-f(_))),{"--n-bezier":x,"--n-button-border-radius":b,"--n-button-box-shadow":i,"--n-button-color":a,"--n-button-width":h,"--n-button-width-pressed":g,"--n-button-height":m,"--n-height":w,"--n-offset":C,"--n-opacity-disabled":t,"--n-rail-border-radius":y,"--n-rail-color":n,"--n-rail-color-active":r,"--n-rail-height":_,"--n-rail-width":v,"--n-width":T,"--n-box-shadow-focus":o,"--n-loading-color":s,"--n-text-color":c,"--n-icon-color":u}}),R=o?v(`switch`,r(()=>d.value[0]),L,e):void 0;return{handleClick:M,handleBlur:P,handleFocus:N,handleKeyup:F,handleKeydown:I,mergedRailStyle:w,pressed:b,mergedClsPrefix:i,mergedValue:g,checked:_,mergedDisabled:m,cssVars:o?void 0:L,themeClass:R?.themeClass,onRender:R?.onRender}},render(){let{mergedClsPrefix:e,mergedDisabled:t,checked:n,mergedRailStyle:r,onRender:a,$slots:c}=this;a?.();let{checked:d,unchecked:f,icon:p,"checked-icon":m,"unchecked-icon":h}=c,g=!(s(p)&&s(m)&&s(h));return i(`div`,{role:`switch`,"aria-checked":n,class:[`${e}-switch`,this.themeClass,g&&`${e}-switch--icon`,n&&`${e}-switch--active`,t&&`${e}-switch--disabled`,this.round&&`${e}-switch--round`,this.loading&&`${e}-switch--loading`,this.pressed&&`${e}-switch--pressed`,this.rubberBand&&`${e}-switch--rubber-band`],tabindex:this.mergedDisabled?void 0:0,style:this.cssVars,onClick:this.handleClick,onFocus:this.handleFocus,onBlur:this.handleBlur,onKeyup:this.handleKeyup,onKeydown:this.handleKeydown},i(`div`,{class:`${e}-switch__rail`,"aria-hidden":`true`,style:r},l(d,t=>l(f,n=>t||n?i(`div`,{"aria-hidden":!0,class:`${e}-switch__children-placeholder`},i(`div`,{class:`${e}-switch__rail-placeholder`},i(`div`,{class:`${e}-switch__button-placeholder`}),t),i(`div`,{class:`${e}-switch__rail-placeholder`},i(`div`,{class:`${e}-switch__button-placeholder`}),n)):null)),i(`div`,{class:`${e}-switch__button`},l(p,t=>l(m,n=>l(h,r=>i(o,null,{default:()=>this.loading?i(u,Object.assign({key:`loading`,clsPrefix:e,strokeWidth:20},this.spinProps)):this.checked&&(n||t)?i(`div`,{class:`${e}-switch__button-icon`,key:n?`checked-icon`:`icon`},n||t):!this.checked&&(r||t)?i(`div`,{class:`${e}-switch__button-icon`,key:r?`unchecked-icon`:`icon`},r||t):null})))),l(d,t=>t&&i(`div`,{key:`checked`,class:`${e}-switch__checked`},t)),l(f,t=>t&&i(`div`,{key:`unchecked`,class:`${e}-switch__unchecked`},t)))))}});export{M as t};