import{$ as e,A as t,At as n,B as r,D as i,T as a,U as o,_ as s,b as c,ht as l,lt as u,st as d,w as f,x as p,y as m}from"./echarts-Bmuv2i_G.js";import{f as h}from"./auth-BTeG-_sB.js";import{I as g,o as _}from"./vue-core-bFnGnCI4.js";import{G as v,K as y,N as b,P as x,Y as S,Z as C,i as w,n as T}from"./light-B4Ik6WNX.js";import{s as E}from"./_plugin-vue_export-helper-BHHysIWo.js";import{n as D,t as O}from"./FormItem-OrlOSWkT.js";import{t as k}from"./DataTable-Kq_YTHzF.js";import{t as A}from"./Input-CJN0Uu5u.js";import{t as j}from"./InputNumber-BlEi_M1y.js";import{t as M}from"./Space-DTuP7eZ3.js";import{t as N}from"./Switch-BydDc8YI.js";import{B as P,H as F,V as I,m as L,p as R,s as z,z as B}from"./index-Yh_aUYzb.js";import{t as V}from"./PageHeader-CdWoZL3J.js";import{t as H}from"./ConfirmDialog-DwLyQC34.js";import{t as U}from"./usePermission-CiOgbdvV.js";import{f as W,h as G,r as K,s as q}from"./dhcp-FbMDtn1f.js";var J={success:t(P,null),error:t(F,null),warning:t(B,null),info:t(I,null)},Y=i({name:`ProgressCircle`,props:{clsPrefix:{type:String,required:!0},status:{type:String,required:!0},strokeWidth:{type:Number,required:!0},fillColor:[String,Object],railColor:String,railStyle:[String,Object],percentage:{type:Number,default:0},offsetDegree:{type:Number,default:0},showIndicator:{type:Boolean,required:!0},indicatorTextColor:String,unit:String,viewBoxWidth:{type:Number,required:!0},gapDegree:{type:Number,required:!0},gapOffsetDegree:{type:Number,default:0}},setup(e,{slots:n}){let r=s(()=>{let t=`gradient`,{fillColor:n}=e;return typeof n==`object`?`${t}-${g(JSON.stringify(n))}`:t});function i(t,n,i,a){let{gapDegree:o,viewBoxWidth:s,strokeWidth:c}=e,l=50+c/2,u=`M ${l},${l} m 0,50
      a 50,50 0 1 1 0,-100
      a 50,50 0 1 1 0,100`,d=Math.PI*2*50;return{pathString:u,pathStyle:{stroke:a===`rail`?i:typeof e.fillColor==`object`?`url(#${r.value})`:i,strokeDasharray:`${Math.min(t,100)/100*(d-o)}px ${s*8}px`,strokeDashoffset:`-${o/2}px`,transformOrigin:n?`center`:void 0,transform:n?`rotate(${n}deg)`:void 0}}}let a=()=>{let n=typeof e.fillColor==`object`,i=n?e.fillColor.stops[0]:``,a=n?e.fillColor.stops[1]:``;return n&&t(`defs`,null,t(`linearGradient`,{id:r.value,x1:`0%`,y1:`100%`,x2:`100%`,y2:`0%`},t(`stop`,{offset:`0%`,"stop-color":i}),t(`stop`,{offset:`100%`,"stop-color":a})))};return()=>{let{fillColor:r,railColor:o,strokeWidth:s,offsetDegree:c,status:l,percentage:u,showIndicator:d,indicatorTextColor:f,unit:p,gapOffsetDegree:m,clsPrefix:h}=e,{pathString:g,pathStyle:_}=i(100,0,o,`rail`),{pathString:v,pathStyle:y}=i(u,c,r,`fill`),b=100+s;return t(`div`,{class:`${h}-progress-content`,role:`none`},t(`div`,{class:`${h}-progress-graph`,"aria-hidden":!0},t(`div`,{class:`${h}-progress-graph-circle`,style:{transform:m?`rotate(${m}deg)`:void 0}},t(`svg`,{viewBox:`0 0 ${b} ${b}`},a(),t(`g`,null,t(`path`,{class:`${h}-progress-graph-circle-rail`,d:g,"stroke-width":s,"stroke-linecap":`round`,fill:`none`,style:_})),t(`g`,null,t(`path`,{class:[`${h}-progress-graph-circle-fill`,u===0&&`${h}-progress-graph-circle-fill--empty`],d:v,"stroke-width":s,"stroke-linecap":`round`,fill:`none`,style:y}))))),d?t(`div`,null,n.default?t(`div`,{class:`${h}-progress-custom-content`,role:`none`},n.default()):l==="default"?t(`div`,{class:`${h}-progress-text`,style:{color:f},role:`none`},t(`span`,{class:`${h}-progress-text__percentage`},u),t(`span`,{class:`${h}-progress-text__unit`},p)):t(`div`,{class:`${h}-progress-icon`,"aria-hidden":!0},t(T,{clsPrefix:h},{default:()=>J[l]}))):null)}}}),X={success:t(P,null),error:t(F,null),warning:t(B,null),info:t(I,null)},Z=i({name:`ProgressLine`,props:{clsPrefix:{type:String,required:!0},percentage:{type:Number,default:0},railColor:String,railStyle:[String,Object],fillColor:[String,Object],status:{type:String,required:!0},indicatorPlacement:{type:String,required:!0},indicatorTextColor:String,unit:{type:String,default:`%`},processing:{type:Boolean,required:!0},showIndicator:{type:Boolean,required:!0},height:[String,Number],railBorderRadius:[String,Number],fillBorderRadius:[String,Number]},setup(e,{slots:n}){let r=s(()=>E(e.height)),i=s(()=>typeof e.fillColor==`object`?`linear-gradient(to right, ${e.fillColor?.stops[0]} , ${e.fillColor?.stops[1]})`:e.fillColor),a=s(()=>e.railBorderRadius===void 0?e.height===void 0?``:E(e.height,{c:.5}):E(e.railBorderRadius)),o=s(()=>e.fillBorderRadius===void 0?e.railBorderRadius===void 0?e.height===void 0?``:E(e.height,{c:.5}):E(e.railBorderRadius):E(e.fillBorderRadius));return()=>{let{indicatorPlacement:s,railColor:c,railStyle:l,percentage:u,unit:d,indicatorTextColor:f,status:p,showIndicator:m,processing:h,clsPrefix:g}=e;return t(`div`,{class:`${g}-progress-content`,role:`none`},t(`div`,{class:`${g}-progress-graph`,"aria-hidden":!0},t(`div`,{class:[`${g}-progress-graph-line`,{[`${g}-progress-graph-line--indicator-${s}`]:!0}]},t(`div`,{class:`${g}-progress-graph-line-rail`,style:[{backgroundColor:c,height:r.value,borderRadius:a.value},l]},t(`div`,{class:[`${g}-progress-graph-line-fill`,h&&`${g}-progress-graph-line-fill--processing`],style:{maxWidth:`${e.percentage}%`,background:i.value,height:r.value,lineHeight:r.value,borderRadius:o.value}},s===`inside`?t(`div`,{class:`${g}-progress-graph-line-indicator`,style:{color:f}},n.default?n.default():`${u}${d}`):null)))),m&&s===`outside`?t(`div`,null,n.default?t(`div`,{class:`${g}-progress-custom-content`,style:{color:f},role:`none`},n.default()):p==="default"?t(`div`,{role:`none`,class:`${g}-progress-icon ${g}-progress-icon--as-text`,style:{color:f}},u,d):t(`div`,{class:`${g}-progress-icon`,"aria-hidden":!0},t(T,{clsPrefix:g},{default:()=>X[p]}))):null)}}});function Q(e,t,n=100){return`m ${n/2} ${n/2-e} a ${e} ${e} 0 1 1 0 ${2*e} a ${e} ${e} 0 1 1 0 -${2*e}`}var $=i({name:`ProgressMultipleCircle`,props:{clsPrefix:{type:String,required:!0},viewBoxWidth:{type:Number,required:!0},percentage:{type:Array,default:[0]},strokeWidth:{type:Number,required:!0},circleGap:{type:Number,required:!0},showIndicator:{type:Boolean,required:!0},fillColor:{type:Array,default:()=>[]},railColor:{type:Array,default:()=>[]},railStyle:{type:Array,default:()=>[]}},setup(e,{slots:n}){let r=s(()=>e.percentage.map((t,n)=>`${Math.PI*t/100*(e.viewBoxWidth/2-e.strokeWidth/2*(1+2*n)-e.circleGap*n)*2}, ${e.viewBoxWidth*8}`)),i=(n,r)=>{let i=e.fillColor[r],a=typeof i==`object`?i.stops[0]:``,o=typeof i==`object`?i.stops[1]:``;return typeof e.fillColor[r]==`object`&&t(`linearGradient`,{id:`gradient-${r}`,x1:`100%`,y1:`0%`,x2:`0%`,y2:`100%`},t(`stop`,{offset:`0%`,"stop-color":a}),t(`stop`,{offset:`100%`,"stop-color":o}))};return()=>{let{viewBoxWidth:a,strokeWidth:o,circleGap:s,showIndicator:c,fillColor:l,railColor:u,railStyle:d,percentage:f,clsPrefix:p}=e;return t(`div`,{class:`${p}-progress-content`,role:`none`},t(`div`,{class:`${p}-progress-graph`,"aria-hidden":!0},t(`div`,{class:`${p}-progress-graph-circle`},t(`svg`,{viewBox:`0 0 ${a} ${a}`},t(`defs`,null,f.map((e,t)=>i(e,t))),f.map((e,n)=>t(`g`,{key:n},t(`path`,{class:`${p}-progress-graph-circle-rail`,d:Q(a/2-o/2*(1+2*n)-s*n,o,a),"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:[{strokeDashoffset:0,stroke:u[n]},d[n]]}),t(`path`,{class:[`${p}-progress-graph-circle-fill`,e===0&&`${p}-progress-graph-circle-fill--empty`],d:Q(a/2-o/2*(1+2*n)-s*n,o,a),"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:{strokeDasharray:r.value[n],strokeDashoffset:0,stroke:typeof l[n]==`object`?`url(#gradient-${n})`:l[n]}})))))),c&&n.default?t(`div`,null,t(`div`,{class:`${p}-progress-text`},n.default())):null)}}}),ee=v([y(`progress`,{display:`inline-block`},[y(`progress-icon`,`
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
 `)]),te=i({name:`Progress`,props:Object.assign(Object.assign({},w.props),{processing:Boolean,type:{type:String,default:`line`},gapDegree:Number,gapOffsetDegree:Number,status:{type:String,default:`default`},railColor:[String,Array],railStyle:[String,Array],color:[String,Array,Object],viewBoxWidth:{type:Number,default:100},strokeWidth:{type:Number,default:7},percentage:[Number,Array],unit:{type:String,default:`%`},showIndicator:{type:Boolean,default:!0},indicatorPosition:{type:String,default:`outside`},indicatorPlacement:{type:String,default:`outside`},indicatorTextColor:String,circleGap:{type:Number,default:1},height:Number,borderRadius:[String,Number],fillBorderRadius:[String,Number],offsetDegree:Number}),setup(e){let t=s(()=>e.indicatorPlacement||e.indicatorPosition),n=s(()=>{if(e.gapDegree||e.gapDegree===0)return e.gapDegree;if(e.type===`dashboard`)return 75}),{mergedClsPrefixRef:r,inlineThemeDisabled:i}=x(e),a=w(`Progress`,`-progress`,ee,z,e,r),o=s(()=>{let{status:t}=e,{common:{cubicBezierEaseInOut:n},self:{fontSize:r,fontSizeCircle:i,railColor:o,railHeight:s,iconSizeCircle:c,iconSizeLine:l,textColorCircle:u,textColorLineInner:d,textColorLineOuter:f,lineBgProcessing:p,fontWeightCircle:m,[C(`iconColor`,t)]:h,[C(`fillColor`,t)]:g}}=a.value;return{"--n-bezier":n,"--n-fill-color":g,"--n-font-size":r,"--n-font-size-circle":i,"--n-font-weight-circle":m,"--n-icon-color":h,"--n-icon-size-circle":c,"--n-icon-size-line":l,"--n-line-bg-processing":p,"--n-rail-color":o,"--n-rail-height":s,"--n-text-color-circle":u,"--n-text-color-line-inner":d,"--n-text-color-line-outer":f}}),c=i?b(`progress`,s(()=>e.status[0]),o,e):void 0;return{mergedClsPrefix:r,mergedIndicatorPlacement:t,gapDeg:n,cssVars:i?void 0:o,themeClass:c?.themeClass,onRender:c?.onRender}},render(){let{type:e,cssVars:n,indicatorTextColor:r,showIndicator:i,status:a,railColor:o,railStyle:s,color:c,percentage:l,viewBoxWidth:u,strokeWidth:d,mergedIndicatorPlacement:f,unit:p,borderRadius:m,fillBorderRadius:h,height:g,processing:_,circleGap:v,mergedClsPrefix:y,gapDeg:b,gapOffsetDegree:x,themeClass:S,$slots:C,onRender:w}=this;return w?.(),t(`div`,{class:[S,`${y}-progress`,`${y}-progress--${e}`,`${y}-progress--${a}`],style:n,"aria-valuemax":100,"aria-valuemin":0,"aria-valuenow":l,role:e===`circle`||e===`line`||e===`dashboard`?`progressbar`:`none`},e===`circle`||e===`dashboard`?t(Y,{clsPrefix:y,status:a,showIndicator:i,indicatorTextColor:r,railColor:o,fillColor:c,railStyle:s,offsetDegree:this.offsetDegree,percentage:l,viewBoxWidth:u,strokeWidth:d,gapDegree:b===void 0?e===`dashboard`?75:0:b,gapOffsetDegree:x,unit:p},C):e===`line`?t(Z,{clsPrefix:y,status:a,showIndicator:i,indicatorTextColor:r,railColor:o,fillColor:c,railStyle:s,percentage:l,processing:_,indicatorPlacement:f,unit:p,fillBorderRadius:h,railBorderRadius:m,height:g},C):e===`multiple-circle`?t($,{clsPrefix:y,strokeWidth:d,railColor:o,fillColor:c,railStyle:s,viewBoxWidth:u,percentage:l,showIndicator:i,circleGap:v},C):null)}}),ne=i({__name:`ScopesView`,setup(i){let{t:s}=_(),g=R(),v=U(),y=u(!1),b=u(!1),x=u([]),S=u(!1),C=u(!1),w=u(``),T=u(null),E=d({page:1,pageSize:20,itemCount:0,showSizePicker:!0,pageSizes:[10,20,50]}),P=d({name:``,subnet:``,start_ip:``,end_ip:``,lease_time:86400,enabled:!0,description:``}),F=[{title:()=>s(`common.name`),key:`name`},{title:()=>s(`dhcp.scopes.subnet`),key:`subnet`},{title:()=>s(`dhcp.scopes.startIp`),key:`start_ip`,width:130},{title:()=>s(`dhcp.scopes.endIp`),key:`end_ip`,width:130},{title:()=>s(`dhcp.scopes.leaseTime`),key:`lease_time`,width:100},{title:()=>s(`dhcp.scopes.activeLeases`),key:`active_leases`,width:100},{title:()=>s(`dhcp.scopes.usage`),key:`usage`,width:120,render:e=>{let n=e.total_addresses>0?Math.round(e.active_leases/e.total_addresses*100):0;return t(te,{type:`line`,percentage:n,indicatorPlacement:`inside`,status:n>90?`error`:n>70?`warning`:`success`})}},{title:()=>s(`common.enabled`),key:`enabled`,width:80,render:e=>t(N,{value:e.enabled,disabled:!v.canWrite(`dhcp`),onUpdateValue:()=>Y(e)})},{title:()=>s(`common.actions`),key:`actions`,width:160,render:e=>t(M,null,{default:()=>[t(h,{size:`small`,onClick:()=>{T.value=e,Object.assign(P,e),S.value=!0}},{default:()=>s(`common.edit`)}),t(h,{size:`small`,type:`error`,disabled:!v.canDelete(`dhcp`),onClick:()=>{w.value=e.id,C.value=!0}},{default:()=>s(`common.delete`)})]})}];async function I(){y.value=!0;try{let e=await W({page:E.page,page_size:E.pageSize});x.value=e.data,E.itemCount=e.meta.total}catch(e){g.error(e instanceof Error?e.message:s(`common.failed`))}finally{y.value=!1}}function z(e){E.page=e,I()}function B(e){E.pageSize=e,E.page=1,I()}function J(){T.value=null,Object.assign(P,{name:``,subnet:``,start_ip:``,end_ip:``,lease_time:86400,enabled:!0,description:``}),S.value=!0}async function Y(e){try{await G(e.id,{enabled:!e.enabled}),g.success(s(`common.updateSuccess`)),I()}catch(e){g.error(e instanceof Error?e.message:s(`common.failed`))}}async function X(){b.value=!0;try{T.value?(await G(T.value.id,P),g.success(s(`common.updateSuccess`))):(await K(P),g.success(s(`common.createSuccess`))),S.value=!1,T.value=null,I()}catch(e){g.error(e instanceof Error?e.message:s(`common.failed`))}finally{b.value=!1}}async function Z(){try{await q(w.value),g.success(s(`common.deleteSuccess`)),I()}catch(e){g.error(e instanceof Error?e.message:s(`common.failed`))}C.value=!1}return r(I),(t,r)=>{let i=k,u=A,d=O,g=j,_=D,w=L;return o(),p(`div`,null,[a(V,{title:l(s)(`dhcp.scopes.title`)},{default:e(()=>[l(v).canWrite(`dhcp`)?(o(),m(l(h),{key:0,type:`primary`,onClick:J},{default:e(()=>[f(n(l(s)(`dhcp.scopes.createScope`)),1)]),_:1})):c(``,!0)]),_:1},8,[`title`]),a(i,{columns:F,data:x.value,loading:y.value,remote:``,pagination:E,"row-key":e=>e.id,"onUpdate:page":z,"onUpdate:pageSize":B},null,8,[`data`,`loading`,`pagination`,`row-key`]),S.value?(o(),m(w,{key:0,show:S.value,"onUpdate:show":r[8]||=e=>S.value=e,preset:`card`,title:T.value?l(s)(`dhcp.scopes.editScope`):l(s)(`dhcp.scopes.createScope`),style:{width:`550px`}},{footer:e(()=>[a(l(M),{justify:`end`},{default:e(()=>[a(l(h),{onClick:r[7]||=e=>S.value=!1},{default:e(()=>[f(n(l(s)(`common.cancel`)),1)]),_:1}),a(l(h),{type:`primary`,loading:b.value,onClick:X},{default:e(()=>[f(n(l(s)(`common.save`)),1)]),_:1},8,[`loading`])]),_:1})]),default:e(()=>[a(_,{model:P,"label-placement":`left`,"label-width":`100px`},{default:e(()=>[a(d,{label:l(s)(`common.name`)},{default:e(()=>[a(u,{value:P.name,"onUpdate:value":r[0]||=e=>P.name=e},null,8,[`value`])]),_:1},8,[`label`]),a(d,{label:l(s)(`dhcp.scopes.subnet`)},{default:e(()=>[a(u,{value:P.subnet,"onUpdate:value":r[1]||=e=>P.subnet=e,placeholder:`192.168.1.0/24`},null,8,[`value`])]),_:1},8,[`label`]),a(d,{label:l(s)(`dhcp.scopes.startIp`)},{default:e(()=>[a(u,{value:P.start_ip,"onUpdate:value":r[2]||=e=>P.start_ip=e},null,8,[`value`])]),_:1},8,[`label`]),a(d,{label:l(s)(`dhcp.scopes.endIp`)},{default:e(()=>[a(u,{value:P.end_ip,"onUpdate:value":r[3]||=e=>P.end_ip=e},null,8,[`value`])]),_:1},8,[`label`]),a(d,{label:l(s)(`dhcp.scopes.leaseTime`)},{default:e(()=>[a(g,{value:P.lease_time,"onUpdate:value":r[4]||=e=>P.lease_time=e,min:60,style:{width:`100%`}},null,8,[`value`])]),_:1},8,[`label`]),a(d,{label:l(s)(`common.description`)},{default:e(()=>[a(u,{value:P.description,"onUpdate:value":r[5]||=e=>P.description=e,type:`textarea`},null,8,[`value`])]),_:1},8,[`label`]),a(d,{label:l(s)(`common.enabled`)},{default:e(()=>[a(l(N),{value:P.enabled,"onUpdate:value":r[6]||=e=>P.enabled=e},null,8,[`value`])]),_:1},8,[`label`])]),_:1},8,[`model`])]),_:1},8,[`show`,`title`])):c(``,!0),a(H,{show:C.value,message:l(s)(`common.deleteConfirm`),onConfirm:Z,onCancel:r[9]||=e=>C.value=!1},null,8,[`show`,`message`])])}}});export{ne as default};