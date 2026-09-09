import{B as e,C as t,E as n,Tt as r,U as i,b as a,dt as o,g as s,k as c,nt as l,pt as u,v as d,w as f,xt as p,y as m}from"./vue.runtime.esm-bundler-B46jYzg4.js";import{$t as h,Dn as g,F as _,H as v,I as y,Ir as b,Jr as x,Kn as S,Nr as C,On as w,Or as T,cn as E,en as D,ht as O,jr as k,kr as A,nn as j,tn as M,un as N,xt as P}from"./router-BPAmgH4V.js";import{n as F,t as I}from"./FormItem-JkVTzYig.js";import{t as L}from"./DataTable-BlEBl1Cw.js";import{t as R}from"./Space-UBzmDyiO.js";import{c as z,i as B}from"./index-D-7w4x16.js";import{t as V}from"./PageHeader-BSmhizjk.js";import{t as H}from"./ConfirmDialog-B4rjVBNZ.js";import{t as U}from"./usePermission-gmBJFweH.js";import{f as W,h as G,r as K,s as q}from"./dhcp-BYV51Gw-.js";var J={success:c(D,null),error:c(j,null),warning:c(h,null),info:c(M,null)},Y=n({name:`ProgressCircle`,props:{clsPrefix:{type:String,required:!0},status:{type:String,required:!0},strokeWidth:{type:Number,required:!0},fillColor:[String,Object],railColor:String,railStyle:[String,Object],percentage:{type:Number,default:0},offsetDegree:{type:Number,default:0},showIndicator:{type:Boolean,required:!0},indicatorTextColor:String,unit:String,viewBoxWidth:{type:Number,required:!0},gapDegree:{type:Number,required:!0},gapOffsetDegree:{type:Number,default:0}},setup(e,{slots:t}){let n=s(()=>{let t=`gradient`,{fillColor:n}=e;return typeof n==`object`?`${t}-${b(JSON.stringify(n))}`:t});function r(t,r,i,a){let{gapDegree:o,viewBoxWidth:s,strokeWidth:c}=e,l=50+c/2,u=`M ${l},${l} m 0,50
      a 50,50 0 1 1 0,-100
      a 50,50 0 1 1 0,100`,d=Math.PI*2*50;return{pathString:u,pathStyle:{stroke:a===`rail`?i:typeof e.fillColor==`object`?`url(#${n.value})`:i,strokeDasharray:`${Math.min(t,100)/100*(d-o)}px ${s*8}px`,strokeDashoffset:`-${o/2}px`,transformOrigin:r?`center`:void 0,transform:r?`rotate(${r}deg)`:void 0}}}let i=()=>{let t=typeof e.fillColor==`object`,r=t?e.fillColor.stops[0]:``,i=t?e.fillColor.stops[1]:``;return t&&c(`defs`,null,c(`linearGradient`,{id:n.value,x1:`0%`,y1:`100%`,x2:`100%`,y2:`0%`},c(`stop`,{offset:`0%`,"stop-color":r}),c(`stop`,{offset:`100%`,"stop-color":i})))};return()=>{let{fillColor:n,railColor:a,strokeWidth:o,offsetDegree:s,status:l,percentage:u,showIndicator:d,indicatorTextColor:f,unit:p,gapOffsetDegree:m,clsPrefix:h}=e,{pathString:g,pathStyle:_}=r(100,0,a,`rail`),{pathString:v,pathStyle:y}=r(u,s,n,`fill`),b=100+o;return c(`div`,{class:`${h}-progress-content`,role:`none`},c(`div`,{class:`${h}-progress-graph`,"aria-hidden":!0},c(`div`,{class:`${h}-progress-graph-circle`,style:{transform:m?`rotate(${m}deg)`:void 0}},c(`svg`,{viewBox:`0 0 ${b} ${b}`},i(),c(`g`,null,c(`path`,{class:`${h}-progress-graph-circle-rail`,d:g,"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:_})),c(`g`,null,c(`path`,{class:[`${h}-progress-graph-circle-fill`,u===0&&`${h}-progress-graph-circle-fill--empty`],d:v,"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:y}))))),d?c(`div`,null,t.default?c(`div`,{class:`${h}-progress-custom-content`,role:`none`},t.default()):l===`default`?c(`div`,{class:`${h}-progress-text`,style:{color:f},role:`none`},c(`span`,{class:`${h}-progress-text__percentage`},u),c(`span`,{class:`${h}-progress-text__unit`},p)):c(`div`,{class:`${h}-progress-icon`,"aria-hidden":!0},c(E,{clsPrefix:h},{default:()=>J[l]}))):null)}}}),X={success:c(D,null),error:c(j,null),warning:c(h,null),info:c(M,null)},Z=n({name:`ProgressLine`,props:{clsPrefix:{type:String,required:!0},percentage:{type:Number,default:0},railColor:String,railStyle:[String,Object],fillColor:[String,Object],status:{type:String,required:!0},indicatorPlacement:{type:String,required:!0},indicatorTextColor:String,unit:{type:String,default:`%`},processing:{type:Boolean,required:!0},showIndicator:{type:Boolean,required:!0},height:[String,Number],railBorderRadius:[String,Number],fillBorderRadius:[String,Number]},setup(e,{slots:t}){let n=s(()=>S(e.height)),r=s(()=>typeof e.fillColor==`object`?`linear-gradient(to right, ${e.fillColor?.stops[0]} , ${e.fillColor?.stops[1]})`:e.fillColor),i=s(()=>e.railBorderRadius===void 0?e.height===void 0?``:S(e.height,{c:.5}):S(e.railBorderRadius)),a=s(()=>e.fillBorderRadius===void 0?e.railBorderRadius===void 0?e.height===void 0?``:S(e.height,{c:.5}):S(e.railBorderRadius):S(e.fillBorderRadius));return()=>{let{indicatorPlacement:o,railColor:s,railStyle:l,percentage:u,unit:d,indicatorTextColor:f,status:p,showIndicator:m,processing:h,clsPrefix:g}=e;return c(`div`,{class:`${g}-progress-content`,role:`none`},c(`div`,{class:`${g}-progress-graph`,"aria-hidden":!0},c(`div`,{class:[`${g}-progress-graph-line`,{[`${g}-progress-graph-line--indicator-${o}`]:!0}]},c(`div`,{class:`${g}-progress-graph-line-rail`,style:[{backgroundColor:s,height:n.value,borderRadius:i.value},l]},c(`div`,{class:[`${g}-progress-graph-line-fill`,h&&`${g}-progress-graph-line-fill--processing`],style:{maxWidth:`${e.percentage}%`,background:r.value,height:n.value,lineHeight:n.value,borderRadius:a.value}},o===`inside`?c(`div`,{class:`${g}-progress-graph-line-indicator`,style:{color:f}},t.default?t.default():`${u}${d}`):null)))),m&&o===`outside`?c(`div`,null,t.default?c(`div`,{class:`${g}-progress-custom-content`,style:{color:f},role:`none`},t.default()):p===`default`?c(`div`,{role:`none`,class:`${g}-progress-icon ${g}-progress-icon--as-text`,style:{color:f}},u,d):c(`div`,{class:`${g}-progress-icon`,"aria-hidden":!0},c(E,{clsPrefix:g},{default:()=>X[p]}))):null)}}});function Q(e,t,n=100){return`m ${n/2} ${n/2-e} a ${e} ${e} 0 1 1 0 ${2*e} a ${e} ${e} 0 1 1 0 -${2*e}`}var $=n({name:`ProgressMultipleCircle`,props:{clsPrefix:{type:String,required:!0},viewBoxWidth:{type:Number,required:!0},percentage:{type:Array,default:[0]},strokeWidth:{type:Number,required:!0},circleGap:{type:Number,required:!0},showIndicator:{type:Boolean,required:!0},fillColor:{type:Array,default:()=>[]},railColor:{type:Array,default:()=>[]},railStyle:{type:Array,default:()=>[]}},setup(e,{slots:t}){let n=s(()=>e.percentage.map((t,n)=>`${Math.PI*t/100*(e.viewBoxWidth/2-e.strokeWidth/2*(1+2*n)-e.circleGap*n)*2}, ${e.viewBoxWidth*8}`)),r=(t,n)=>{let r=e.fillColor[n],i=typeof r==`object`?r.stops[0]:``,a=typeof r==`object`?r.stops[1]:``;return typeof e.fillColor[n]==`object`&&c(`linearGradient`,{id:`gradient-${n}`,x1:`100%`,y1:`0%`,x2:`0%`,y2:`100%`},c(`stop`,{offset:`0%`,"stop-color":i}),c(`stop`,{offset:`100%`,"stop-color":a}))};return()=>{let{viewBoxWidth:i,strokeWidth:a,circleGap:o,showIndicator:s,fillColor:l,railColor:u,railStyle:d,percentage:f,clsPrefix:p}=e;return c(`div`,{class:`${p}-progress-content`,role:`none`},c(`div`,{class:`${p}-progress-graph`,"aria-hidden":!0},c(`div`,{class:`${p}-progress-graph-circle`},c(`svg`,{viewBox:`0 0 ${i} ${i}`},c(`defs`,null,f.map((e,t)=>r(e,t))),f.map((e,t)=>c(`g`,{key:t},c(`path`,{class:`${p}-progress-graph-circle-rail`,d:Q(i/2-a/2*(1+2*t)-o*t,a,i),"stroke-width":a,"stroke-linecap":`round`,fill:`none`,style:[{strokeDashoffset:0,stroke:u[t]},d[t]]}),c(`path`,{class:[`${p}-progress-graph-circle-fill`,e===0&&`${p}-progress-graph-circle-fill--empty`],d:Q(i/2-a/2*(1+2*t)-o*t,a,i),"stroke-width":a,"stroke-linecap":`round`,fill:`none`,style:{strokeDasharray:n.value[t],strokeDashoffset:0,stroke:typeof l[t]==`object`?`url(#gradient-${t})`:l[t]}})))))),s&&t.default?c(`div`,null,c(`div`,{class:`${p}-progress-text`},t.default())):null)}}}),ee=T([A(`progress`,{display:`inline-block`},[A(`progress-icon`,`
 color: var(--n-icon-color);
 transition: color .3s var(--n-bezier);
 `),k(`line`,`
 width: 100%;
 display: block;
 `,[A(`progress-content`,`
 display: flex;
 align-items: center;
 `,[A(`progress-graph`,{flex:1})]),A(`progress-custom-content`,{marginLeft:`14px`}),A(`progress-icon`,`
 width: 30px;
 padding-left: 14px;
 height: var(--n-icon-size-line);
 line-height: var(--n-icon-size-line);
 font-size: var(--n-icon-size-line);
 `,[k(`as-text`,`
 color: var(--n-text-color-line-outer);
 text-align: center;
 width: 40px;
 font-size: var(--n-font-size);
 padding-left: 4px;
 transition: color .3s var(--n-bezier);
 `)])]),k(`circle, dashboard`,{width:`120px`},[A(`progress-custom-content`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 justify-content: center;
 `),A(`progress-text`,`
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
 `),A(`progress-icon`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 color: var(--n-icon-color);
 font-size: var(--n-icon-size-circle);
 `)]),k(`multiple-circle`,`
 width: 200px;
 color: inherit;
 `,[A(`progress-text`,`
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
 `)]),A(`progress-content`,{position:`relative`}),A(`progress-graph`,{position:`relative`},[A(`progress-graph-circle`,[T(`svg`,{verticalAlign:`bottom`}),A(`progress-graph-circle-fill`,`
 stroke: var(--n-fill-color);
 transition:
 opacity .3s var(--n-bezier),
 stroke .3s var(--n-bezier),
 stroke-dasharray .3s var(--n-bezier);
 `,[k(`empty`,{opacity:0})]),A(`progress-graph-circle-rail`,`
 transition: stroke .3s var(--n-bezier);
 overflow: hidden;
 stroke: var(--n-rail-color);
 `)]),A(`progress-graph-line`,[k(`indicator-inside`,[A(`progress-graph-line-rail`,`
 height: 16px;
 line-height: 16px;
 border-radius: 10px;
 `,[A(`progress-graph-line-fill`,`
 height: inherit;
 border-radius: 10px;
 `),A(`progress-graph-line-indicator`,`
 background: #0000;
 white-space: nowrap;
 text-align: right;
 margin-left: 14px;
 margin-right: 14px;
 height: inherit;
 font-size: 12px;
 color: var(--n-text-color-line-inner);
 transition: color .3s var(--n-bezier);
 `)])]),k(`indicator-inside-label`,`
 height: 16px;
 display: flex;
 align-items: center;
 `,[A(`progress-graph-line-rail`,`
 flex: 1;
 transition: background-color .3s var(--n-bezier);
 `),A(`progress-graph-line-indicator`,`
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
 `)]),A(`progress-graph-line-rail`,`
 position: relative;
 overflow: hidden;
 height: var(--n-rail-height);
 border-radius: 5px;
 background-color: var(--n-rail-color);
 transition: background-color .3s var(--n-bezier);
 `,[A(`progress-graph-line-fill`,`
 background: var(--n-fill-color);
 position: relative;
 border-radius: 5px;
 height: inherit;
 width: 100%;
 max-width: 0%;
 transition:
 background-color .3s var(--n-bezier),
 max-width .2s var(--n-bezier);
 `,[k(`processing`,[T(`&::after`,`
 content: "";
 background-image: var(--n-line-bg-processing);
 animation: progress-processing-animation 2s var(--n-bezier) infinite;
 `)])])])])])]),T(`@keyframes progress-processing-animation`,`
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
 `)]),te=n({name:`Progress`,props:Object.assign(Object.assign({},N.props),{processing:Boolean,type:{type:String,default:`line`},gapDegree:Number,gapOffsetDegree:Number,status:{type:String,default:`default`},railColor:[String,Array],railStyle:[String,Array],color:[String,Array,Object],viewBoxWidth:{type:Number,default:100},strokeWidth:{type:Number,default:7},percentage:[Number,Array],unit:{type:String,default:`%`},showIndicator:{type:Boolean,default:!0},indicatorPosition:{type:String,default:`outside`},indicatorPlacement:{type:String,default:`outside`},indicatorTextColor:String,circleGap:{type:Number,default:1},height:Number,borderRadius:[String,Number],fillBorderRadius:[String,Number],offsetDegree:Number}),setup(e){let t=s(()=>e.indicatorPlacement||e.indicatorPosition),n=s(()=>{if(e.gapDegree||e.gapDegree===0)return e.gapDegree;if(e.type===`dashboard`)return 75}),{mergedClsPrefixRef:r,inlineThemeDisabled:i}=w(e),a=N(`Progress`,`-progress`,ee,B,e,r),o=s(()=>{let{status:t}=e,{common:{cubicBezierEaseInOut:n},self:{fontSize:r,fontSizeCircle:i,railColor:o,railHeight:s,iconSizeCircle:c,iconSizeLine:l,textColorCircle:u,textColorLineInner:d,textColorLineOuter:f,lineBgProcessing:p,fontWeightCircle:m,[C(`iconColor`,t)]:h,[C(`fillColor`,t)]:g}}=a.value;return{"--n-bezier":n,"--n-fill-color":g,"--n-font-size":r,"--n-font-size-circle":i,"--n-font-weight-circle":m,"--n-icon-color":h,"--n-icon-size-circle":c,"--n-icon-size-line":l,"--n-line-bg-processing":p,"--n-rail-color":o,"--n-rail-height":s,"--n-text-color-circle":u,"--n-text-color-line-inner":d,"--n-text-color-line-outer":f}}),c=i?g(`progress`,s(()=>e.status[0]),o,e):void 0;return{mergedClsPrefix:r,mergedIndicatorPlacement:t,gapDeg:n,cssVars:i?void 0:o,themeClass:c?.themeClass,onRender:c?.onRender}},render(){let{type:e,cssVars:t,indicatorTextColor:n,showIndicator:r,status:i,railColor:a,railStyle:o,color:s,percentage:l,viewBoxWidth:u,strokeWidth:d,mergedIndicatorPlacement:f,unit:p,borderRadius:m,fillBorderRadius:h,height:g,processing:_,circleGap:v,mergedClsPrefix:y,gapDeg:b,gapOffsetDegree:x,themeClass:S,$slots:C,onRender:w}=this;return w?.(),c(`div`,{class:[S,`${y}-progress`,`${y}-progress--${e}`,`${y}-progress--${i}`],style:t,"aria-valuemax":100,"aria-valuemin":0,"aria-valuenow":l,role:e===`circle`||e===`line`||e===`dashboard`?`progressbar`:`none`},e===`circle`||e===`dashboard`?c(Y,{clsPrefix:y,status:i,showIndicator:r,indicatorTextColor:n,railColor:a,fillColor:s,railStyle:o,offsetDegree:this.offsetDegree,percentage:l,viewBoxWidth:u,strokeWidth:d,gapDegree:b===void 0?e===`dashboard`?75:0:b,gapOffsetDegree:x,unit:p},C):e===`line`?c(Z,{clsPrefix:y,status:i,showIndicator:r,indicatorTextColor:n,railColor:a,fillColor:s,railStyle:o,percentage:l,processing:_,indicatorPlacement:f,unit:p,fillBorderRadius:h,railBorderRadius:m,height:g},C):e===`multiple-circle`?c($,{clsPrefix:y,strokeWidth:d,railColor:a,fillColor:s,railStyle:o,viewBoxWidth:u,percentage:l,showIndicator:r,circleGap:v},C):null)}}),ne=n({name:`dhcp_scopes`,__name:`index`,setup(n){let{t:s}=x(),h=z(),g=U(),b=u(!1),S=u(!1),C=u([]),w=u(!1),T=u(!1),E=u(``),D=u(null),k=o({page:1,pageSize:20,itemCount:0,showSizePicker:!0,pageSizes:[10,20,50]}),A=o({name:``,subnet:``,start_ip:``,end_ip:``,lease_time:86400,enabled:!0,description:``}),j=[{title:()=>s(`common.name`),key:`name`},{title:()=>s(`dhcp.scopes.subnet`),key:`subnet`},{title:()=>s(`dhcp.scopes.startIp`),key:`start_ip`,width:130},{title:()=>s(`dhcp.scopes.endIp`),key:`end_ip`,width:130},{title:()=>s(`dhcp.scopes.leaseTime`),key:`lease_time`,width:100},{title:()=>s(`dhcp.scopes.activeLeases`),key:`active_leases`,width:100},{title:()=>s(`dhcp.scopes.usage`),key:`usage`,width:120,render:e=>{let t=e.total_addresses>0?Math.round(e.active_leases/e.total_addresses*100):0;return c(te,{type:`line`,percentage:t,indicatorPlacement:`inside`,status:t>90?`error`:t>70?`warning`:`success`})}},{title:()=>s(`common.enabled`),key:`enabled`,width:80,render:e=>c(_,{value:e.enabled,disabled:!g.canWrite(`dhcp`),onUpdateValue:()=>Y(e)})},{title:()=>s(`common.actions`),key:`actions`,width:160,render:e=>c(R,null,{default:()=>[c(O,{size:`small`,text:!0,onClick:()=>{D.value=e,Object.assign(A,e),w.value=!0}},{default:()=>s(`common.edit`)}),c(O,{size:`small`,text:!0,type:`error`,disabled:!g.canDelete(`dhcp`),onClick:()=>{E.value=e.id,T.value=!0}},{default:()=>s(`common.delete`)})]})}];async function M(){b.value=!0;try{let e=await W({page:k.page,page_size:k.pageSize});C.value=e.data,k.itemCount=e.meta.total}catch(e){h.error(e instanceof Error?e.message:s(`common.failed`))}finally{b.value=!1}}function N(e){k.page=e,M()}function B(e){k.pageSize=e,k.page=1,M()}function J(){D.value=null,Object.assign(A,{name:``,subnet:``,start_ip:``,end_ip:``,lease_time:86400,enabled:!0,description:``}),w.value=!0}async function Y(e){try{await G(e.id,{enabled:!e.enabled}),h.success(s(`common.updateSuccess`)),M()}catch(e){h.error(e instanceof Error?e.message:s(`common.failed`))}}async function X(){S.value=!0;try{D.value?(await G(D.value.id,A),h.success(s(`common.updateSuccess`))):(await K(A),h.success(s(`common.createSuccess`))),w.value=!1,D.value=null,M()}catch(e){h.error(e instanceof Error?e.message:s(`common.failed`))}finally{S.value=!1}}async function Z(){try{await q(E.value),h.success(s(`common.deleteSuccess`)),M()}catch(e){h.error(e instanceof Error?e.message:s(`common.failed`))}T.value=!1}return e(M),(e,n)=>{let o=L,c=P,u=I,h=y,x=F,E=v;return i(),a(`div`,null,[f(V,{title:p(s)(`dhcp.scopes.title`)},{default:l(()=>[p(g).canWrite(`dhcp`)?(i(),d(p(O),{key:0,type:`primary`,onClick:J},{default:l(()=>[t(r(p(s)(`dhcp.scopes.createScope`)),1)]),_:1})):m(``,!0)]),_:1},8,[`title`]),f(o,{columns:j,data:C.value,loading:b.value,remote:``,pagination:k,"row-key":e=>e.id,"onUpdate:page":N,"onUpdate:pageSize":B},null,8,[`data`,`loading`,`pagination`,`row-key`]),w.value?(i(),d(E,{key:0,show:w.value,"onUpdate:show":n[8]||=e=>w.value=e,preset:`card`,title:D.value?p(s)(`dhcp.scopes.editScope`):p(s)(`dhcp.scopes.createScope`),style:{width:`550px`}},{footer:l(()=>[f(p(R),{justify:`end`},{default:l(()=>[f(p(O),{onClick:n[7]||=e=>w.value=!1},{default:l(()=>[t(r(p(s)(`common.cancel`)),1)]),_:1}),f(p(O),{type:`primary`,loading:S.value,onClick:X},{default:l(()=>[t(r(p(s)(`common.save`)),1)]),_:1},8,[`loading`])]),_:1})]),default:l(()=>[f(x,{model:A,"label-placement":`left`,"label-width":`100px`},{default:l(()=>[f(u,{label:p(s)(`common.name`)},{default:l(()=>[f(c,{value:A.name,"onUpdate:value":n[0]||=e=>A.name=e},null,8,[`value`])]),_:1},8,[`label`]),f(u,{label:p(s)(`dhcp.scopes.subnet`)},{default:l(()=>[f(c,{value:A.subnet,"onUpdate:value":n[1]||=e=>A.subnet=e,placeholder:`192.168.1.0/24`},null,8,[`value`])]),_:1},8,[`label`]),f(u,{label:p(s)(`dhcp.scopes.startIp`)},{default:l(()=>[f(c,{value:A.start_ip,"onUpdate:value":n[2]||=e=>A.start_ip=e},null,8,[`value`])]),_:1},8,[`label`]),f(u,{label:p(s)(`dhcp.scopes.endIp`)},{default:l(()=>[f(c,{value:A.end_ip,"onUpdate:value":n[3]||=e=>A.end_ip=e},null,8,[`value`])]),_:1},8,[`label`]),f(u,{label:p(s)(`dhcp.scopes.leaseTime`)},{default:l(()=>[f(h,{value:A.lease_time,"onUpdate:value":n[4]||=e=>A.lease_time=e,min:60,style:{width:`100%`}},null,8,[`value`])]),_:1},8,[`label`]),f(u,{label:p(s)(`common.description`)},{default:l(()=>[f(c,{value:A.description,"onUpdate:value":n[5]||=e=>A.description=e,type:`textarea`},null,8,[`value`])]),_:1},8,[`label`]),f(u,{label:p(s)(`common.enabled`)},{default:l(()=>[f(p(_),{value:A.enabled,"onUpdate:value":n[6]||=e=>A.enabled=e},null,8,[`value`])]),_:1},8,[`label`])]),_:1},8,[`model`])]),_:1},8,[`show`,`title`])):m(``,!0),f(H,{show:T.value,message:p(s)(`common.deleteConfirm`),onConfirm:Z,onCancel:n[9]||=e=>T.value=!1},null,8,[`show`,`message`])])}}});export{ne as default};