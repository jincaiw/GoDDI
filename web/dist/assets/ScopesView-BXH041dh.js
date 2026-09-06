<<<<<<<< HEAD:web/dist/assets/ScopesView-BXH041dh.js
import{E as e,O as t,S as n,T as r,V as i,W as a,b as o,ct as s,et as c,gt as l,j as u,jt as d,ut as f,v as p,x as m}from"./echarts-eUEtiXc8.js";import{f as h}from"./auth-BqKin8Qo.js";import{I as g,o as _}from"./vue-core-CcEAoDdX.js";import{G as v,K as y,N as b,P as x,Y as S,Z as C,i as w,n as T}from"./light-BKCENzy2.js";import{s as E}from"./_plugin-vue_export-helper-CteSXPyx.js";import{n as D,t as O}from"./FormItem-DVMP-O3z.js";import{t as k}from"./DataTable-bgnU0Ck1.js";import{t as A}from"./Input-QiGIhbIe.js";import{t as j}from"./InputNumber-B0AcIU2n.js";import{t as M}from"./Space-C-GWV-zf.js";import{t as N}from"./Switch-DxAcw-Ui.js";import{B as P,H as F,V as I,m as L,p as R,s as z,z as B}from"./index-BFRegr9_.js";import{t as V}from"./PageHeader-D_ZqAl_v.js";import{t as H}from"./ConfirmDialog-DoHnCzXy.js";import{t as U}from"./usePermission-DCnzFeIn.js";import{f as W,h as G,r as K,s as q}from"./dhcp-oGJwzm0W.js";var J={success:u(P,null),error:u(F,null),warning:u(B,null),info:u(I,null)},Y=t({name:`ProgressCircle`,props:{clsPrefix:{type:String,required:!0},status:{type:String,required:!0},strokeWidth:{type:Number,required:!0},fillColor:[String,Object],railColor:String,railStyle:[String,Object],percentage:{type:Number,default:0},offsetDegree:{type:Number,default:0},showIndicator:{type:Boolean,required:!0},indicatorTextColor:String,unit:String,viewBoxWidth:{type:Number,required:!0},gapDegree:{type:Number,required:!0},gapOffsetDegree:{type:Number,default:0}},setup(e,{slots:t}){let n=p(()=>{let t=`gradient`,{fillColor:n}=e;return typeof n==`object`?`${t}-${g(JSON.stringify(n))}`:t});function r(t,r,i,a){let{gapDegree:o,viewBoxWidth:s,strokeWidth:c}=e,l=50+c/2,u=`M ${l},${l} m 0,50
========
import{C as e,E as t,H as n,Q as r,b as i,ct as a,g as o,k as s,kt as c,mt as l,ot as u,v as d,w as f,y as p,z as m}from"./echarts-Cw2yHLaZ.js";import{f as h}from"./auth-RfiMf961.js";import{I as g,o as _}from"./vue-core-BDTvi3xZ.js";import{G as v,K as y,N as b,P as x,Y as S,Z as C,i as w,n as T}from"./light-BluJl8SD.js";import{o as E}from"./get-BLwATRqs.js";import{n as D,t as O}from"./FormItem-CV_32by-.js";import{t as k}from"./DataTable-CGxHs8lR.js";import{t as A}from"./Input-DdipMcL-.js";import{t as j}from"./InputNumber-C_0YnVnY.js";import{t as M}from"./Space-NJnKprsS.js";import{t as N}from"./Switch-hIX5t2y-.js";import{B as P,H as F,V as I,m as L,p as R,s as z,z as B}from"./index-CRgX9Dvo.js";import{t as V}from"./PageHeader-DA_8GL16.js";import{t as H}from"./ConfirmDialog-DHA9KEXI.js";import{t as U}from"./usePermission-DabgMDkv.js";import{f as W,h as G,r as K,s as q}from"./dhcp-CBkSycSP.js";var J={success:s(P,null),error:s(F,null),warning:s(B,null),info:s(I,null)},Y=t({name:`ProgressCircle`,props:{clsPrefix:{type:String,required:!0},status:{type:String,required:!0},strokeWidth:{type:Number,required:!0},fillColor:[String,Object],railColor:String,railStyle:[String,Object],percentage:{type:Number,default:0},offsetDegree:{type:Number,default:0},showIndicator:{type:Boolean,required:!0},indicatorTextColor:String,unit:String,viewBoxWidth:{type:Number,required:!0},gapDegree:{type:Number,required:!0},gapOffsetDegree:{type:Number,default:0}},setup(e,{slots:t}){let n=o(()=>{let t=`gradient`,{fillColor:n}=e;return typeof n==`object`?`${t}-${g(JSON.stringify(n))}`:t});function r(t,r,i,a){let{gapDegree:o,viewBoxWidth:s,strokeWidth:c}=e,l=50+c/2,u=`M ${l},${l} m 0,50
>>>>>>>> e53a2cc (feat: redesign console UI (dark+light design system) and add session management):web/dist/assets/ScopesView-D_P4fpRg.js
      a 50,50 0 1 1 0,-100
      a 50,50 0 1 1 0,100`,d=Math.PI*2*50;return{pathString:u,pathStyle:{stroke:a===`rail`?i:typeof e.fillColor==`object`?`url(#${n.value})`:i,strokeDasharray:`${Math.min(t,100)/100*(d-o)}px ${s*8}px`,strokeDashoffset:`-${o/2}px`,transformOrigin:r?`center`:void 0,transform:r?`rotate(${r}deg)`:void 0}}}let i=()=>{let t=typeof e.fillColor==`object`,r=t?e.fillColor.stops[0]:``,i=t?e.fillColor.stops[1]:``;return t&&u(`defs`,null,u(`linearGradient`,{id:n.value,x1:`0%`,y1:`100%`,x2:`100%`,y2:`0%`},u(`stop`,{offset:`0%`,"stop-color":r}),u(`stop`,{offset:`100%`,"stop-color":i})))};return()=>{let{fillColor:n,railColor:a,strokeWidth:o,offsetDegree:s,status:c,percentage:l,showIndicator:d,indicatorTextColor:f,unit:p,gapOffsetDegree:m,clsPrefix:h}=e,{pathString:g,pathStyle:_}=r(100,0,a,`rail`),{pathString:v,pathStyle:y}=r(l,s,n,`fill`),b=100+o;return u(`div`,{class:`${h}-progress-content`,role:`none`},u(`div`,{class:`${h}-progress-graph`,"aria-hidden":!0},u(`div`,{class:`${h}-progress-graph-circle`,style:{transform:m?`rotate(${m}deg)`:void 0}},u(`svg`,{viewBox:`0 0 ${b} ${b}`},i(),u(`g`,null,u(`path`,{class:`${h}-progress-graph-circle-rail`,d:g,"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:_})),u(`g`,null,u(`path`,{class:[`${h}-progress-graph-circle-fill`,l===0&&`${h}-progress-graph-circle-fill--empty`],d:v,"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:y}))))),d?u(`div`,null,t.default?u(`div`,{class:`${h}-progress-custom-content`,role:`none`},t.default()):c==="default"?u(`div`,{class:`${h}-progress-text`,style:{color:f},role:`none`},u(`span`,{class:`${h}-progress-text__percentage`},l),u(`span`,{class:`${h}-progress-text__unit`},p)):u(`div`,{class:`${h}-progress-icon`,"aria-hidden":!0},u(T,{clsPrefix:h},{default:()=>J[c]}))):null)}}}),X={success:u(P,null),error:u(F,null),warning:u(B,null),info:u(I,null)},Z=t({name:`ProgressLine`,props:{clsPrefix:{type:String,required:!0},percentage:{type:Number,default:0},railColor:String,railStyle:[String,Object],fillColor:[String,Object],status:{type:String,required:!0},indicatorPlacement:{type:String,required:!0},indicatorTextColor:String,unit:{type:String,default:`%`},processing:{type:Boolean,required:!0},showIndicator:{type:Boolean,required:!0},height:[String,Number],railBorderRadius:[String,Number],fillBorderRadius:[String,Number]},setup(e,{slots:t}){let n=p(()=>E(e.height)),r=p(()=>typeof e.fillColor==`object`?`linear-gradient(to right, ${e.fillColor?.stops[0]} , ${e.fillColor?.stops[1]})`:e.fillColor),i=p(()=>e.railBorderRadius===void 0?e.height===void 0?``:E(e.height,{c:.5}):E(e.railBorderRadius)),a=p(()=>e.fillBorderRadius===void 0?e.railBorderRadius===void 0?e.height===void 0?``:E(e.height,{c:.5}):E(e.railBorderRadius):E(e.fillBorderRadius));return()=>{let{indicatorPlacement:o,railColor:s,railStyle:c,percentage:l,unit:d,indicatorTextColor:f,status:p,showIndicator:m,processing:h,clsPrefix:g}=e;return u(`div`,{class:`${g}-progress-content`,role:`none`},u(`div`,{class:`${g}-progress-graph`,"aria-hidden":!0},u(`div`,{class:[`${g}-progress-graph-line`,{[`${g}-progress-graph-line--indicator-${o}`]:!0}]},u(`div`,{class:`${g}-progress-graph-line-rail`,style:[{backgroundColor:s,height:n.value,borderRadius:i.value},c]},u(`div`,{class:[`${g}-progress-graph-line-fill`,h&&`${g}-progress-graph-line-fill--processing`],style:{maxWidth:`${e.percentage}%`,background:r.value,height:n.value,lineHeight:n.value,borderRadius:a.value}},o===`inside`?u(`div`,{class:`${g}-progress-graph-line-indicator`,style:{color:f}},t.default?t.default():`${l}${d}`):null)))),m&&o===`outside`?u(`div`,null,t.default?u(`div`,{class:`${g}-progress-custom-content`,style:{color:f},role:`none`},t.default()):p==="default"?u(`div`,{role:`none`,class:`${g}-progress-icon ${g}-progress-icon--as-text`,style:{color:f}},l,d):u(`div`,{class:`${g}-progress-icon`,"aria-hidden":!0},u(T,{clsPrefix:g},{default:()=>X[p]}))):null)}}});function Q(e,t,n=100){return`m ${n/2} ${n/2-e} a ${e} ${e} 0 1 1 0 ${2*e} a ${e} ${e} 0 1 1 0 -${2*e}`}var $=t({name:`ProgressMultipleCircle`,props:{clsPrefix:{type:String,required:!0},viewBoxWidth:{type:Number,required:!0},percentage:{type:Array,default:[0]},strokeWidth:{type:Number,required:!0},circleGap:{type:Number,required:!0},showIndicator:{type:Boolean,required:!0},fillColor:{type:Array,default:()=>[]},railColor:{type:Array,default:()=>[]},railStyle:{type:Array,default:()=>[]}},setup(e,{slots:t}){let n=p(()=>e.percentage.map((t,n)=>`${Math.PI*t/100*(e.viewBoxWidth/2-e.strokeWidth/2*(1+2*n)-e.circleGap*n)*2}, ${e.viewBoxWidth*8}`)),r=(t,n)=>{let r=e.fillColor[n],i=typeof r==`object`?r.stops[0]:``,a=typeof r==`object`?r.stops[1]:``;return typeof e.fillColor[n]==`object`&&u(`linearGradient`,{id:`gradient-${n}`,x1:`100%`,y1:`0%`,x2:`0%`,y2:`100%`},u(`stop`,{offset:`0%`,"stop-color":i}),u(`stop`,{offset:`100%`,"stop-color":a}))};return()=>{let{viewBoxWidth:i,strokeWidth:a,circleGap:o,showIndicator:s,fillColor:c,railColor:l,railStyle:d,percentage:f,clsPrefix:p}=e;return u(`div`,{class:`${p}-progress-content`,role:`none`},u(`div`,{class:`${p}-progress-graph`,"aria-hidden":!0},u(`div`,{class:`${p}-progress-graph-circle`},u(`svg`,{viewBox:`0 0 ${i} ${i}`},u(`defs`,null,f.map((e,t)=>r(e,t))),f.map((e,t)=>u(`g`,{key:t},u(`path`,{class:`${p}-progress-graph-circle-rail`,d:Q(i/2-a/2*(1+2*t)-o*t,a,i),"stroke-width":a,"stroke-linecap":`round`,fill:`none`,style:[{strokeDashoffset:0,stroke:l[t]},d[t]]}),u(`path`,{class:[`${p}-progress-graph-circle-fill`,e===0&&`${p}-progress-graph-circle-fill--empty`],d:Q(i/2-a/2*(1+2*t)-o*t,a,i),"stroke-width":a,"stroke-linecap":`round`,fill:`none`,style:{strokeDasharray:n.value[t],strokeDashoffset:0,stroke:typeof c[t]==`object`?`url(#gradient-${t})`:c[t]}})))))),s&&t.default?u(`div`,null,u(`div`,{class:`${p}-progress-text`},t.default())):null)}}}),ee=v([y(`progress`,{display:`inline-block`},[y(`progress-icon`,`
 color: var(--n-icon-color);
 transition: color .3s var(--n-bezier);
 `),S(`line`,`
 width: 100%;
 display: block;
 `,[y(`progress-content`,`
 display: flex;
 align-items: center;
 `,[y(`progress-graph`,{flex:1})]),y(`progress-custom-content`,{marginLeft:`14px`}),y(`progress-icon`,`
 width: 30px;
 padding-left: 14px;
 height: var(--n-icon-size-line);
 line-height: var(--n-icon-size-line);
 font-size: var(--n-icon-size-line);
 `,[S(`as-text`,`
 color: var(--n-text-color-line-outer);
 text-align: center;
 width: 40px;
 font-size: var(--n-font-size);
 padding-left: 4px;
 transition: color .3s var(--n-bezier);
 `)])]),S(`circle, dashboard`,{width:`120px`},[y(`progress-custom-content`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 justify-content: center;
 `),y(`progress-text`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 color: inherit;
 font-size: var(--n-font-size-circle);
 color: var(--n-text-color-circle);
 font-weight: var(--n-font-weight-circle);
 transition: color .3s var(--n-bezier);
 white-space: nowrap;
 `),y(`progress-icon`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 color: var(--n-icon-color);
 font-size: var(--n-icon-size-circle);
 `)]),S(`multiple-circle`,`
 width: 200px;
 color: inherit;
 `,[y(`progress-text`,`
 font-weight: var(--n-font-weight-circle);
 color: var(--n-text-color-circle);
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 justify-content: center;
 transition: color .3s var(--n-bezier);
 `)]),y(`progress-content`,{position:`relative`}),y(`progress-graph`,{position:`relative`},[y(`progress-graph-circle`,[v(`svg`,{verticalAlign:`bottom`}),y(`progress-graph-circle-fill`,`
 stroke: var(--n-fill-color);
 transition:
 opacity .3s var(--n-bezier),
 stroke .3s var(--n-bezier),
 stroke-dasharray .3s var(--n-bezier);
 `,[S(`empty`,{opacity:0})]),y(`progress-graph-circle-rail`,`
 transition: stroke .3s var(--n-bezier);
 overflow: hidden;
 stroke: var(--n-rail-color);
 `)]),y(`progress-graph-line`,[S(`indicator-inside`,[y(`progress-graph-line-rail`,`
 height: 16px;
 line-height: 16px;
 border-radius: 10px;
 `,[y(`progress-graph-line-fill`,`
 height: inherit;
 border-radius: 10px;
 `),y(`progress-graph-line-indicator`,`
 background: #0000;
 white-space: nowrap;
 text-align: right;
 margin-left: 14px;
 margin-right: 14px;
 height: inherit;
 font-size: 12px;
 color: var(--n-text-color-line-inner);
 transition: color .3s var(--n-bezier);
 `)])]),S(`indicator-inside-label`,`
 height: 16px;
 display: flex;
 align-items: center;
 `,[y(`progress-graph-line-rail`,`
 flex: 1;
 transition: background-color .3s var(--n-bezier);
 `),y(`progress-graph-line-indicator`,`
 background: var(--n-fill-color);
 font-size: 12px;
 transform: translateZ(0);
 display: flex;
 vertical-align: middle;
 height: 16px;
 line-height: 16px;
 padding: 0 10px;
 border-radius: 10px;
 position: absolute;
 white-space: nowrap;
 color: var(--n-text-color-line-inner);
 transition:
 right .2s var(--n-bezier),
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `)]),y(`progress-graph-line-rail`,`
 position: relative;
 overflow: hidden;
 height: var(--n-rail-height);
 border-radius: 5px;
 background-color: var(--n-rail-color);
 transition: background-color .3s var(--n-bezier);
 `,[y(`progress-graph-line-fill`,`
 background: var(--n-fill-color);
 position: relative;
 border-radius: 5px;
 height: inherit;
 width: 100%;
 max-width: 0%;
 transition:
 background-color .3s var(--n-bezier),
 max-width .2s var(--n-bezier);
 `,[S(`processing`,[v(`&::after`,`
 content: "";
 background-image: var(--n-line-bg-processing);
 animation: progress-processing-animation 2s var(--n-bezier) infinite;
 `)])])])])])]),v(`@keyframes progress-processing-animation`,`
 0% {
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 right: 100%;
 opacity: 1;
 }
 66% {
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 right: 0;
 opacity: 0;
 }
 100% {
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 right: 0;
 opacity: 0;
 }
<<<<<<<< HEAD:web/dist/assets/ScopesView-BXH041dh.js
 `)]),te=t({name:`Progress`,props:Object.assign(Object.assign({},w.props),{processing:Boolean,type:{type:String,default:`line`},gapDegree:Number,gapOffsetDegree:Number,status:{type:String,default:`default`},railColor:[String,Array],railStyle:[String,Array],color:[String,Array,Object],viewBoxWidth:{type:Number,default:100},strokeWidth:{type:Number,default:7},percentage:[Number,Array],unit:{type:String,default:`%`},showIndicator:{type:Boolean,default:!0},indicatorPosition:{type:String,default:`outside`},indicatorPlacement:{type:String,default:`outside`},indicatorTextColor:String,circleGap:{type:Number,default:1},height:Number,borderRadius:[String,Number],fillBorderRadius:[String,Number],offsetDegree:Number}),setup(e){let t=p(()=>e.indicatorPlacement||e.indicatorPosition),n=p(()=>{if(e.gapDegree||e.gapDegree===0)return e.gapDegree;if(e.type===`dashboard`)return 75}),{mergedClsPrefixRef:r,inlineThemeDisabled:i}=x(e),a=w(`Progress`,`-progress`,ee,z,e,r),o=p(()=>{let{status:t}=e,{common:{cubicBezierEaseInOut:n},self:{fontSize:r,fontSizeCircle:i,railColor:o,railHeight:s,iconSizeCircle:c,iconSizeLine:l,textColorCircle:u,textColorLineInner:d,textColorLineOuter:f,lineBgProcessing:p,fontWeightCircle:m,[C(`iconColor`,t)]:h,[C(`fillColor`,t)]:g}}=a.value;return{"--n-bezier":n,"--n-fill-color":g,"--n-font-size":r,"--n-font-size-circle":i,"--n-font-weight-circle":m,"--n-icon-color":h,"--n-icon-size-circle":c,"--n-icon-size-line":l,"--n-line-bg-processing":p,"--n-rail-color":o,"--n-rail-height":s,"--n-text-color-circle":u,"--n-text-color-line-inner":d,"--n-text-color-line-outer":f}}),s=i?b(`progress`,p(()=>e.status[0]),o,e):void 0;return{mergedClsPrefix:r,mergedIndicatorPlacement:t,gapDeg:n,cssVars:i?void 0:o,themeClass:s?.themeClass,onRender:s?.onRender}},render(){let{type:e,cssVars:t,indicatorTextColor:n,showIndicator:r,status:i,railColor:a,railStyle:o,color:s,percentage:c,viewBoxWidth:l,strokeWidth:d,mergedIndicatorPlacement:f,unit:p,borderRadius:m,fillBorderRadius:h,height:g,processing:_,circleGap:v,mergedClsPrefix:y,gapDeg:b,gapOffsetDegree:x,themeClass:S,$slots:C,onRender:w}=this;return w?.(),u(`div`,{class:[S,`${y}-progress`,`${y}-progress--${e}`,`${y}-progress--${i}`],style:t,"aria-valuemax":100,"aria-valuemin":0,"aria-valuenow":c,role:e===`circle`||e===`line`||e===`dashboard`?`progressbar`:`none`},e===`circle`||e===`dashboard`?u(Y,{clsPrefix:y,status:i,showIndicator:r,indicatorTextColor:n,railColor:a,fillColor:s,railStyle:o,offsetDegree:this.offsetDegree,percentage:c,viewBoxWidth:l,strokeWidth:d,gapDegree:b===void 0?e===`dashboard`?75:0:b,gapOffsetDegree:x,unit:p},C):e===`line`?u(Z,{clsPrefix:y,status:i,showIndicator:r,indicatorTextColor:n,railColor:a,fillColor:s,railStyle:o,percentage:c,processing:_,indicatorPlacement:f,unit:p,fillBorderRadius:h,railBorderRadius:m,height:g},C):e===`multiple-circle`?u($,{clsPrefix:y,strokeWidth:d,railColor:a,fillColor:s,railStyle:o,viewBoxWidth:l,percentage:c,showIndicator:r,circleGap:v},C):null)}}),ne=t({__name:`ScopesView`,setup(t){let{t:p}=_(),g=R(),v=U(),y=f(!1),b=f(!1),x=f([]),S=f(!1),C=f(!1),w=f(``),T=f(null),E=s({page:1,pageSize:20,itemCount:0,showSizePicker:!0,pageSizes:[10,20,50]}),P=s({name:``,subnet:``,start_ip:``,end_ip:``,lease_time:86400,enabled:!0,description:``}),F=[{title:()=>p(`common.name`),key:`name`},{title:()=>p(`dhcp.scopes.subnet`),key:`subnet`},{title:()=>p(`dhcp.scopes.startIp`),key:`start_ip`,width:130},{title:()=>p(`dhcp.scopes.endIp`),key:`end_ip`,width:130},{title:()=>p(`dhcp.scopes.leaseTime`),key:`lease_time`,width:100},{title:()=>p(`dhcp.scopes.activeLeases`),key:`active_leases`,width:100},{title:()=>p(`dhcp.scopes.usage`),key:`usage`,width:120,render:e=>{let t=e.total_addresses>0?Math.round(e.active_leases/e.total_addresses*100):0;return u(te,{type:`line`,percentage:t,indicatorPlacement:`inside`,status:t>90?`error`:t>70?`warning`:`success`})}},{title:()=>p(`common.enabled`),key:`enabled`,width:80,render:e=>u(N,{value:e.enabled,disabled:!v.canWrite(`dhcp`),onUpdateValue:()=>Y(e)})},{title:()=>p(`common.actions`),key:`actions`,width:160,render:e=>u(M,null,{default:()=>[u(h,{size:`small`,onClick:()=>{T.value=e,Object.assign(P,e),S.value=!0}},{default:()=>p(`common.edit`)}),u(h,{size:`small`,type:`error`,disabled:!v.canDelete(`dhcp`),onClick:()=>{w.value=e.id,C.value=!0}},{default:()=>p(`common.delete`)})]})}];async function I(){y.value=!0;try{let e=await W({page:E.page,page_size:E.pageSize});x.value=e.data,E.itemCount=e.meta.total}catch(e){g.error(e instanceof Error?e.message:p(`common.failed`))}finally{y.value=!1}}function z(e){E.page=e,I()}function B(e){E.pageSize=e,E.page=1,I()}function J(){T.value=null,Object.assign(P,{name:``,subnet:``,start_ip:``,end_ip:``,lease_time:86400,enabled:!0,description:``}),S.value=!0}async function Y(e){try{await G(e.id,{enabled:!e.enabled}),g.success(p(`common.updateSuccess`)),I()}catch(e){g.error(e instanceof Error?e.message:p(`common.failed`))}}async function X(){b.value=!0;try{T.value?(await G(T.value.id,P),g.success(p(`common.updateSuccess`))):(await K(P),g.success(p(`common.createSuccess`))),S.value=!1,T.value=null,I()}catch(e){g.error(e instanceof Error?e.message:p(`common.failed`))}finally{b.value=!1}}async function Z(){try{await q(w.value),g.success(p(`common.deleteSuccess`)),I()}catch(e){g.error(e instanceof Error?e.message:p(`common.failed`))}C.value=!1}return i(I),(t,i)=>{let s=k,u=A,f=O,g=j,_=D,w=L;return a(),n(`div`,null,[e(V,{title:l(p)(`dhcp.scopes.title`)},{default:c(()=>[l(v).canWrite(`dhcp`)?(a(),o(l(h),{key:0,type:`primary`,onClick:J},{default:c(()=>[r(d(l(p)(`dhcp.scopes.createScope`)),1)]),_:1})):m(``,!0)]),_:1},8,[`title`]),e(s,{columns:F,data:x.value,loading:y.value,remote:``,pagination:E,"row-key":e=>e.id,"onUpdate:page":z,"onUpdate:pageSize":B},null,8,[`data`,`loading`,`pagination`,`row-key`]),S.value?(a(),o(w,{key:0,show:S.value,"onUpdate:show":i[8]||=e=>S.value=e,preset:`card`,title:T.value?l(p)(`dhcp.scopes.editScope`):l(p)(`dhcp.scopes.createScope`),style:{width:`550px`}},{footer:c(()=>[e(l(M),{justify:`end`},{default:c(()=>[e(l(h),{onClick:i[7]||=e=>S.value=!1},{default:c(()=>[r(d(l(p)(`common.cancel`)),1)]),_:1}),e(l(h),{type:`primary`,loading:b.value,onClick:X},{default:c(()=>[r(d(l(p)(`common.save`)),1)]),_:1},8,[`loading`])]),_:1})]),default:c(()=>[e(_,{model:P,"label-placement":`left`,"label-width":`100px`},{default:c(()=>[e(f,{label:l(p)(`common.name`)},{default:c(()=>[e(u,{value:P.name,"onUpdate:value":i[0]||=e=>P.name=e},null,8,[`value`])]),_:1},8,[`label`]),e(f,{label:l(p)(`dhcp.scopes.subnet`)},{default:c(()=>[e(u,{value:P.subnet,"onUpdate:value":i[1]||=e=>P.subnet=e,placeholder:`192.168.1.0/24`},null,8,[`value`])]),_:1},8,[`label`]),e(f,{label:l(p)(`dhcp.scopes.startIp`)},{default:c(()=>[e(u,{value:P.start_ip,"onUpdate:value":i[2]||=e=>P.start_ip=e},null,8,[`value`])]),_:1},8,[`label`]),e(f,{label:l(p)(`dhcp.scopes.endIp`)},{default:c(()=>[e(u,{value:P.end_ip,"onUpdate:value":i[3]||=e=>P.end_ip=e},null,8,[`value`])]),_:1},8,[`label`]),e(f,{label:l(p)(`dhcp.scopes.leaseTime`)},{default:c(()=>[e(g,{value:P.lease_time,"onUpdate:value":i[4]||=e=>P.lease_time=e,min:60,style:{width:`100%`}},null,8,[`value`])]),_:1},8,[`label`]),e(f,{label:l(p)(`common.description`)},{default:c(()=>[e(u,{value:P.description,"onUpdate:value":i[5]||=e=>P.description=e,type:`textarea`},null,8,[`value`])]),_:1},8,[`label`]),e(f,{label:l(p)(`common.enabled`)},{default:c(()=>[e(l(N),{value:P.enabled,"onUpdate:value":i[6]||=e=>P.enabled=e},null,8,[`value`])]),_:1},8,[`label`])]),_:1},8,[`model`])]),_:1},8,[`show`,`title`])):m(``,!0),e(H,{show:C.value,message:l(p)(`common.deleteConfirm`),onConfirm:Z,onCancel:i[9]||=e=>C.value=!1},null,8,[`show`,`message`])])}}});export{ne as default};
========
 `)]),te=t({name:`Progress`,props:Object.assign(Object.assign({},w.props),{processing:Boolean,type:{type:String,default:`line`},gapDegree:Number,gapOffsetDegree:Number,status:{type:String,default:`default`},railColor:[String,Array],railStyle:[String,Array],color:[String,Array,Object],viewBoxWidth:{type:Number,default:100},strokeWidth:{type:Number,default:7},percentage:[Number,Array],unit:{type:String,default:`%`},showIndicator:{type:Boolean,default:!0},indicatorPosition:{type:String,default:`outside`},indicatorPlacement:{type:String,default:`outside`},indicatorTextColor:String,circleGap:{type:Number,default:1},height:Number,borderRadius:[String,Number],fillBorderRadius:[String,Number],offsetDegree:Number}),setup(e){let t=o(()=>e.indicatorPlacement||e.indicatorPosition),n=o(()=>{if(e.gapDegree||e.gapDegree===0)return e.gapDegree;if(e.type===`dashboard`)return 75}),{mergedClsPrefixRef:r,inlineThemeDisabled:i}=x(e),a=w(`Progress`,`-progress`,ee,z,e,r),s=o(()=>{let{status:t}=e,{common:{cubicBezierEaseInOut:n},self:{fontSize:r,fontSizeCircle:i,railColor:o,railHeight:s,iconSizeCircle:c,iconSizeLine:l,textColorCircle:u,textColorLineInner:d,textColorLineOuter:f,lineBgProcessing:p,fontWeightCircle:m,[C(`iconColor`,t)]:h,[C(`fillColor`,t)]:g}}=a.value;return{"--n-bezier":n,"--n-fill-color":g,"--n-font-size":r,"--n-font-size-circle":i,"--n-font-weight-circle":m,"--n-icon-color":h,"--n-icon-size-circle":c,"--n-icon-size-line":l,"--n-line-bg-processing":p,"--n-rail-color":o,"--n-rail-height":s,"--n-text-color-circle":u,"--n-text-color-line-inner":d,"--n-text-color-line-outer":f}}),c=i?b(`progress`,o(()=>e.status[0]),s,e):void 0;return{mergedClsPrefix:r,mergedIndicatorPlacement:t,gapDeg:n,cssVars:i?void 0:s,themeClass:c?.themeClass,onRender:c?.onRender}},render(){let{type:e,cssVars:t,indicatorTextColor:n,showIndicator:r,status:i,railColor:a,railStyle:o,color:c,percentage:l,viewBoxWidth:u,strokeWidth:d,mergedIndicatorPlacement:f,unit:p,borderRadius:m,fillBorderRadius:h,height:g,processing:_,circleGap:v,mergedClsPrefix:y,gapDeg:b,gapOffsetDegree:x,themeClass:S,$slots:C,onRender:w}=this;return w?.(),s(`div`,{class:[S,`${y}-progress`,`${y}-progress--${e}`,`${y}-progress--${i}`],style:t,"aria-valuemax":100,"aria-valuemin":0,"aria-valuenow":l,role:e===`circle`||e===`line`||e===`dashboard`?`progressbar`:`none`},e===`circle`||e===`dashboard`?s(Y,{clsPrefix:y,status:i,showIndicator:r,indicatorTextColor:n,railColor:a,fillColor:c,railStyle:o,offsetDegree:this.offsetDegree,percentage:l,viewBoxWidth:u,strokeWidth:d,gapDegree:b===void 0?e===`dashboard`?75:0:b,gapOffsetDegree:x,unit:p},C):e===`line`?s(Z,{clsPrefix:y,status:i,showIndicator:r,indicatorTextColor:n,railColor:a,fillColor:c,railStyle:o,percentage:l,processing:_,indicatorPlacement:f,unit:p,fillBorderRadius:h,railBorderRadius:m,height:g},C):e===`multiple-circle`?s($,{clsPrefix:y,strokeWidth:d,railColor:a,fillColor:c,railStyle:o,viewBoxWidth:u,percentage:l,showIndicator:r,circleGap:v},C):null)}}),ne=t({__name:`ScopesView`,setup(t){let{t:o}=_(),g=R(),v=U(),y=a(!1),b=a(!1),x=a([]),S=a(!1),C=a(!1),w=a(``),T=a(null),E=u({page:1,pageSize:20,itemCount:0,showSizePicker:!0,pageSizes:[10,20,50]}),P=u({name:``,subnet:``,start_ip:``,end_ip:``,lease_time:86400,enabled:!0,description:``}),F=[{title:()=>o(`common.name`),key:`name`},{title:()=>o(`dhcp.scopes.subnet`),key:`subnet`},{title:()=>o(`dhcp.scopes.startIp`),key:`start_ip`,width:130},{title:()=>o(`dhcp.scopes.endIp`),key:`end_ip`,width:130},{title:()=>o(`dhcp.scopes.leaseTime`),key:`lease_time`,width:100},{title:()=>o(`dhcp.scopes.activeLeases`),key:`active_leases`,width:100},{title:()=>o(`dhcp.scopes.usage`),key:`usage`,width:120,render:e=>{let t=e.total_addresses>0?Math.round(e.active_leases/e.total_addresses*100):0;return s(te,{type:`line`,percentage:t,indicatorPlacement:`inside`,status:t>90?`error`:t>70?`warning`:`success`})}},{title:()=>o(`common.enabled`),key:`enabled`,width:80,render:e=>s(N,{value:e.enabled,disabled:!v.canWrite(`dhcp`),onUpdateValue:()=>Y(e)})},{title:()=>o(`common.actions`),key:`actions`,width:160,render:e=>s(M,null,{default:()=>[s(h,{size:`small`,text:!0,onClick:()=>{T.value=e,Object.assign(P,e),S.value=!0}},{default:()=>o(`common.edit`)}),s(h,{size:`small`,text:!0,type:`error`,disabled:!v.canDelete(`dhcp`),onClick:()=>{w.value=e.id,C.value=!0}},{default:()=>o(`common.delete`)})]})}];async function I(){y.value=!0;try{let e=await W({page:E.page,page_size:E.pageSize});x.value=e.data,E.itemCount=e.meta.total}catch(e){g.error(e instanceof Error?e.message:o(`common.failed`))}finally{y.value=!1}}function z(e){E.page=e,I()}function B(e){E.pageSize=e,E.page=1,I()}function J(){T.value=null,Object.assign(P,{name:``,subnet:``,start_ip:``,end_ip:``,lease_time:86400,enabled:!0,description:``}),S.value=!0}async function Y(e){try{await G(e.id,{enabled:!e.enabled}),g.success(o(`common.updateSuccess`)),I()}catch(e){g.error(e instanceof Error?e.message:o(`common.failed`))}}async function X(){b.value=!0;try{T.value?(await G(T.value.id,P),g.success(o(`common.updateSuccess`))):(await K(P),g.success(o(`common.createSuccess`))),S.value=!1,T.value=null,I()}catch(e){g.error(e instanceof Error?e.message:o(`common.failed`))}finally{b.value=!1}}async function Z(){try{await q(w.value),g.success(o(`common.deleteSuccess`)),I()}catch(e){g.error(e instanceof Error?e.message:o(`common.failed`))}C.value=!1}return m(I),(t,a)=>{let s=k,u=A,m=O,g=j,_=D,w=L;return n(),i(`div`,null,[f(V,{title:l(o)(`dhcp.scopes.title`)},{default:r(()=>[l(v).canWrite(`dhcp`)?(n(),d(l(h),{key:0,type:`primary`,onClick:J},{default:r(()=>[e(c(l(o)(`dhcp.scopes.createScope`)),1)]),_:1})):p(``,!0)]),_:1},8,[`title`]),f(s,{columns:F,data:x.value,loading:y.value,remote:``,pagination:E,"row-key":e=>e.id,"onUpdate:page":z,"onUpdate:pageSize":B},null,8,[`data`,`loading`,`pagination`,`row-key`]),S.value?(n(),d(w,{key:0,show:S.value,"onUpdate:show":a[8]||=e=>S.value=e,preset:`card`,title:T.value?l(o)(`dhcp.scopes.editScope`):l(o)(`dhcp.scopes.createScope`),style:{width:`550px`}},{footer:r(()=>[f(l(M),{justify:`end`},{default:r(()=>[f(l(h),{onClick:a[7]||=e=>S.value=!1},{default:r(()=>[e(c(l(o)(`common.cancel`)),1)]),_:1}),f(l(h),{type:`primary`,loading:b.value,onClick:X},{default:r(()=>[e(c(l(o)(`common.save`)),1)]),_:1},8,[`loading`])]),_:1})]),default:r(()=>[f(_,{model:P,"label-placement":`left`,"label-width":`100px`},{default:r(()=>[f(m,{label:l(o)(`common.name`)},{default:r(()=>[f(u,{value:P.name,"onUpdate:value":a[0]||=e=>P.name=e},null,8,[`value`])]),_:1},8,[`label`]),f(m,{label:l(o)(`dhcp.scopes.subnet`)},{default:r(()=>[f(u,{value:P.subnet,"onUpdate:value":a[1]||=e=>P.subnet=e,placeholder:`192.168.1.0/24`},null,8,[`value`])]),_:1},8,[`label`]),f(m,{label:l(o)(`dhcp.scopes.startIp`)},{default:r(()=>[f(u,{value:P.start_ip,"onUpdate:value":a[2]||=e=>P.start_ip=e},null,8,[`value`])]),_:1},8,[`label`]),f(m,{label:l(o)(`dhcp.scopes.endIp`)},{default:r(()=>[f(u,{value:P.end_ip,"onUpdate:value":a[3]||=e=>P.end_ip=e},null,8,[`value`])]),_:1},8,[`label`]),f(m,{label:l(o)(`dhcp.scopes.leaseTime`)},{default:r(()=>[f(g,{value:P.lease_time,"onUpdate:value":a[4]||=e=>P.lease_time=e,min:60,style:{width:`100%`}},null,8,[`value`])]),_:1},8,[`label`]),f(m,{label:l(o)(`common.description`)},{default:r(()=>[f(u,{value:P.description,"onUpdate:value":a[5]||=e=>P.description=e,type:`textarea`},null,8,[`value`])]),_:1},8,[`label`]),f(m,{label:l(o)(`common.enabled`)},{default:r(()=>[f(l(N),{value:P.enabled,"onUpdate:value":a[6]||=e=>P.enabled=e},null,8,[`value`])]),_:1},8,[`label`])]),_:1},8,[`model`])]),_:1},8,[`show`,`title`])):p(``,!0),f(H,{show:C.value,message:l(o)(`common.deleteConfirm`),onConfirm:Z,onCancel:a[9]||=e=>C.value=!1},null,8,[`show`,`message`])])}}});export{ne as default};
>>>>>>>> e53a2cc (feat: redesign console UI (dark+light design system) and add session management):web/dist/assets/ScopesView-D_P4fpRg.js
