import{O as e,T as t,dt as n,g as r,st as i}from"./echarts-DxBJA66o.js";import{E as a,It as o,Lt as s,Nt as c,Ot as l,Pt as u,Rt as d,b as f,dt as p,gt as m,k as h,lt as g,ut as _,v,w as y,xt as b,yt as x,zt as S}from"./auth-DEMSBBAb.js";import{O as C,j as w}from"./vue-core-RsSxNVS3.js";import{s as T}from"./get-D5hjkymV.js";import{i as E}from"./index-lJkVLtdC.js";function D(e){let{primaryColor:t,opacityDisabled:n,borderRadius:r,textColor3:i}=e;return Object.assign(Object.assign({},E),{iconColor:i,textColor:`white`,loadingColor:t,opacityDisabled:n,railColor:`rgba(0, 0, 0, .14)`,railColorActive:t,buttonBoxShadow:`0 1px 4px 0 rgba(0, 0, 0, 0.3), inset 0 0 1px 0 rgba(0, 0, 0, 0.05)`,buttonColor:`#FFF`,railBorderRadiusSmall:r,railBorderRadiusMedium:r,railBorderRadiusLarge:r,buttonBorderRadiusSmall:r,buttonBorderRadiusMedium:r,buttonBorderRadiusLarge:r,boxShadowFocus:`0 0 0 2px ${l(t,{alpha:.2})}`})}var O={name:`Switch`,common:v,self:D},k=u(`switch`,`
 height: var(--n-height);
 min-width: var(--n-width);
 vertical-align: middle;
 user-select: none;
 -webkit-user-select: none;
 display: inline-flex;
 outline: none;
 justify-content: center;
 align-items: center;
`,[o(`children-placeholder`,`
 height: var(--n-rail-height);
 display: flex;
 flex-direction: column;
 overflow: hidden;
 pointer-events: none;
 visibility: hidden;
 `),o(`rail-placeholder`,`
 display: flex;
 flex-wrap: none;
 `),o(`button-placeholder`,`
 width: calc(1.75 * var(--n-rail-height));
 height: var(--n-rail-height);
 `),u(`base-loading`,`
 position: absolute;
 top: 50%;
 left: 50%;
 transform: translateX(-50%) translateY(-50%);
 font-size: calc(var(--n-button-width) - 4px);
 color: var(--n-loading-color);
 transition: color .3s var(--n-bezier);
 `,[y({left:`50%`,top:`50%`,originalTransform:`translateX(-50%) translateY(-50%)`})]),o(`checked, unchecked`,`
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
 `),o(`checked`,`
 right: 0;
 padding-right: calc(1.25 * var(--n-rail-height) - var(--n-offset));
 `),o(`unchecked`,`
 left: 0;
 justify-content: flex-end;
 padding-left: calc(1.25 * var(--n-rail-height) - var(--n-offset));
 `),c(`&:focus`,[o(`rail`,`
 box-shadow: var(--n-box-shadow-focus);
 `)]),s(`round`,[o(`rail`,`border-radius: calc(var(--n-rail-height) / 2);`,[o(`button`,`border-radius: calc(var(--n-button-height) / 2);`)])]),d(`disabled`,[d(`icon`,[s(`rubber-band`,[s(`pressed`,[o(`rail`,[o(`button`,`max-width: var(--n-button-width-pressed);`)])]),o(`rail`,[c(`&:active`,[o(`button`,`max-width: var(--n-button-width-pressed);`)])]),s(`active`,[s(`pressed`,[o(`rail`,[o(`button`,`left: calc(100% - var(--n-offset) - var(--n-button-width-pressed));`)])]),o(`rail`,[c(`&:active`,[o(`button`,`left: calc(100% - var(--n-offset) - var(--n-button-width-pressed));`)])])])])])]),s(`active`,[o(`rail`,[o(`button`,`left: calc(100% - var(--n-button-width) - var(--n-offset))`)])]),o(`rail`,`
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
 `,[o(`button-icon`,`
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
 `,[y()]),o(`button`,`
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
 `)]),s(`active`,[o(`rail`,`background-color: var(--n-rail-color-active);`)]),s(`loading`,[o(`rail`,`
 cursor: wait;
 `)]),s(`disabled`,[o(`rail`,`
 cursor: not-allowed;
 opacity: .5;
 `)])]),A=Object.assign(Object.assign({},h.props),{size:String,value:{type:[String,Number,Boolean],default:void 0},loading:Boolean,defaultValue:{type:[String,Number,Boolean],default:!1},disabled:{type:Boolean,default:void 0},round:{type:Boolean,default:!0},"onUpdate:value":[Function,Array],onUpdateValue:[Function,Array],checkedValue:{type:[String,Number,Boolean],default:!0},uncheckedValue:{type:[String,Number,Boolean],default:!1},railStyle:Function,rubberBand:{type:Boolean,default:!0},spinProps:Object,onChange:[Function,Array]}),j,M=t({name:`Switch`,props:A,slots:Object,setup(e){j===void 0&&(j=typeof CSS<`u`?CSS.supports===void 0?!1:CSS.supports(`width`,`max(1px)`):!0);let{mergedClsPrefixRef:t,inlineThemeDisabled:a,mergedComponentPropsRef:o}=p(e),s=h(`Switch`,`-switch`,k,O,e,t),c=g(e,{mergedSize(t){return e.size===void 0?t?t.mergedSize.value:o?.value?.Switch?.size||`medium`:e.size}}),{mergedSizeRef:l,mergedDisabledRef:u}=c,d=i(e.defaultValue),f=T(n(e,`value`),d),m=r(()=>f.value===e.checkedValue),v=i(!1),y=i(!1),x=r(()=>{let{railStyle:t}=e;if(t)return t({focused:y.value,checked:m.value})});function E(t){let{"onUpdate:value":n,onChange:r,onUpdateValue:i}=e,{nTriggerFormInput:a,nTriggerFormChange:o}=c;n&&b(n,t),i&&b(i,t),r&&b(r,t),d.value=t,a(),o()}function D(){let{nTriggerFormFocus:e}=c;e()}function A(){let{nTriggerFormBlur:e}=c;e()}function M(){e.loading||u.value||(f.value===e.checkedValue?E(e.uncheckedValue):E(e.checkedValue))}function N(){y.value=!0,D()}function P(){y.value=!1,A(),v.value=!1}function F(t){e.loading||u.value||t.key===` `&&(f.value===e.checkedValue?E(e.uncheckedValue):E(e.checkedValue),v.value=!1)}function I(t){e.loading||u.value||t.key===` `&&(t.preventDefault(),v.value=!0)}let L=r(()=>{let{value:e}=l,{self:{opacityDisabled:t,railColor:n,railColorActive:r,buttonBoxShadow:i,buttonColor:a,boxShadowFocus:o,loadingColor:c,textColor:u,iconColor:d,[S(`buttonHeight`,e)]:f,[S(`buttonWidth`,e)]:p,[S(`buttonWidthPressed`,e)]:m,[S(`railHeight`,e)]:h,[S(`railWidth`,e)]:g,[S(`railBorderRadius`,e)]:_,[S(`buttonBorderRadius`,e)]:v},common:{cubicBezierEaseInOut:y}}=s.value,b,x,T;return j?(b=`calc((${h} - ${f}) / 2)`,x=`max(${h}, ${f})`,T=`max(${g}, calc(${g} + ${f} - ${h}))`):(b=w((C(h)-C(f))/2),x=w(Math.max(C(h),C(f))),T=C(h)>C(f)?g:w(C(g)+C(f)-C(h))),{"--n-bezier":y,"--n-button-border-radius":v,"--n-button-box-shadow":i,"--n-button-color":a,"--n-button-width":p,"--n-button-width-pressed":m,"--n-button-height":f,"--n-height":x,"--n-offset":b,"--n-opacity-disabled":t,"--n-rail-border-radius":_,"--n-rail-color":n,"--n-rail-color-active":r,"--n-rail-height":h,"--n-rail-width":g,"--n-width":T,"--n-box-shadow-focus":o,"--n-loading-color":c,"--n-text-color":u,"--n-icon-color":d}}),R=a?_(`switch`,r(()=>l.value[0]),L,e):void 0;return{handleClick:M,handleBlur:P,handleFocus:N,handleKeyup:F,handleKeydown:I,mergedRailStyle:x,pressed:v,mergedClsPrefix:t,mergedValue:f,checked:m,mergedDisabled:u,cssVars:a?void 0:L,themeClass:R?.themeClass,onRender:R?.onRender}},render(){let{mergedClsPrefix:t,mergedDisabled:n,checked:r,mergedRailStyle:i,onRender:o,$slots:s}=this;o?.();let{checked:c,unchecked:l,icon:u,"checked-icon":d,"unchecked-icon":p}=s,h=!(m(u)&&m(d)&&m(p));return e(`div`,{role:`switch`,"aria-checked":r,class:[`${t}-switch`,this.themeClass,h&&`${t}-switch--icon`,r&&`${t}-switch--active`,n&&`${t}-switch--disabled`,this.round&&`${t}-switch--round`,this.loading&&`${t}-switch--loading`,this.pressed&&`${t}-switch--pressed`,this.rubberBand&&`${t}-switch--rubber-band`],tabindex:this.mergedDisabled?void 0:0,style:this.cssVars,onClick:this.handleClick,onFocus:this.handleFocus,onBlur:this.handleBlur,onKeyup:this.handleKeyup,onKeydown:this.handleKeydown},e(`div`,{class:`${t}-switch__rail`,"aria-hidden":`true`,style:i},x(c,n=>x(l,r=>n||r?e(`div`,{"aria-hidden":!0,class:`${t}-switch__children-placeholder`},e(`div`,{class:`${t}-switch__rail-placeholder`},e(`div`,{class:`${t}-switch__button-placeholder`}),n),e(`div`,{class:`${t}-switch__rail-placeholder`},e(`div`,{class:`${t}-switch__button-placeholder`}),r)):null)),e(`div`,{class:`${t}-switch__button`},x(u,n=>x(d,r=>x(p,i=>e(a,null,{default:()=>this.loading?e(f,Object.assign({key:`loading`,clsPrefix:t,strokeWidth:20},this.spinProps)):this.checked&&(r||n)?e(`div`,{class:`${t}-switch__button-icon`,key:r?`checked-icon`:`icon`},r||n):!this.checked&&(i||n)?e(`div`,{class:`${t}-switch__button-icon`,key:i?`unchecked-icon`:`icon`},i||n):null})))),x(c,n=>n&&e(`div`,{key:`checked`,class:`${t}-switch__checked`},n)),x(l,n=>n&&e(`div`,{key:`unchecked`,class:`${t}-switch__unchecked`},n)))))}});export{M as t};