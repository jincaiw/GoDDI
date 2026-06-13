import{C as e,O as t,Ot as n,R as r,T as i,V as a,Z as o,at as s,b as c,g as l,pt as u,st as d,v as f,w as p,y as m}from"./echarts-DxBJA66o.js";import{D as h,Lt as g,Nt as _,Pt as v,dt as y,k as b,l as x,ut as S,zt as C}from"./auth-DEMSBBAb.js";import{I as w,o as T}from"./vue-core-RsSxNVS3.js";import{o as E}from"./get-D5hjkymV.js";import{n as D,t as O}from"./FormItem-BoUwzbFY.js";import{t as k}from"./DataTable-CrcfT3Tc.js";import{t as A}from"./Input-B3_w6SX1.js";import{t as j}from"./InputNumber-CyIzv-Z5.js";import{t as M}from"./use-message-CwVCvVSw.js";import{t as N}from"./Space-CCsiZ15l.js";import{t as P}from"./Switch-CFCaDdRr.js";import{B as F,H as I,V as L,m as R,s as z,z as B}from"./index-lJkVLtdC.js";import{t as V}from"./PageHeader-BAc0kMfG.js";import{t as H}from"./ConfirmDialog-DJng3LwZ.js";import{t as U}from"./usePermission-D6zZ2yUQ.js";import{f as W,h as G,r as K,s as q}from"./dhcp-HeyYP6XL.js";var J={success:t(F,null),error:t(I,null),warning:t(B,null),info:t(L,null)},Y=i({name:`ProgressCircle`,props:{clsPrefix:{type:String,required:!0},status:{type:String,required:!0},strokeWidth:{type:Number,required:!0},fillColor:[String,Object],railColor:String,railStyle:[String,Object],percentage:{type:Number,default:0},offsetDegree:{type:Number,default:0},showIndicator:{type:Boolean,required:!0},indicatorTextColor:String,unit:String,viewBoxWidth:{type:Number,required:!0},gapDegree:{type:Number,required:!0},gapOffsetDegree:{type:Number,default:0}},setup(e,{slots:n}){let r=l(()=>{let t=`gradient`,{fillColor:n}=e;return typeof n==`object`?`${t}-${w(JSON.stringify(n))}`:t});function i(t,n,i,a){let{gapDegree:o,viewBoxWidth:s,strokeWidth:c}=e,l=50+c/2,u=`M ${l},${l} m 0,50
      a 50,50 0 1 1 0,-100
      a 50,50 0 1 1 0,100`,d=Math.PI*2*50;return{pathString:u,pathStyle:{stroke:a===`rail`?i:typeof e.fillColor==`object`?`url(#${r.value})`:i,strokeDasharray:`${Math.min(t,100)/100*(d-o)}px ${s*8}px`,strokeDashoffset:`-${o/2}px`,transformOrigin:n?`center`:void 0,transform:n?`rotate(${n}deg)`:void 0}}}let a=()=>{let n=typeof e.fillColor==`object`,i=n?e.fillColor.stops[0]:``,a=n?e.fillColor.stops[1]:``;return n&&t(`defs`,null,t(`linearGradient`,{id:r.value,x1:`0%`,y1:`100%`,x2:`100%`,y2:`0%`},t(`stop`,{offset:`0%`,"stop-color":i}),t(`stop`,{offset:`100%`,"stop-color":a})))};return()=>{let{fillColor:r,railColor:o,strokeWidth:s,offsetDegree:c,status:l,percentage:u,showIndicator:d,indicatorTextColor:f,unit:p,gapOffsetDegree:m,clsPrefix:g}=e,{pathString:_,pathStyle:v}=i(100,0,o,`rail`),{pathString:y,pathStyle:b}=i(u,c,r,`fill`),x=100+s;return t(`div`,{class:`${g}-progress-content`,role:`none`},t(`div`,{class:`${g}-progress-graph`,"aria-hidden":!0},t(`div`,{class:`${g}-progress-graph-circle`,style:{transform:m?`rotate(${m}deg)`:void 0}},t(`svg`,{viewBox:`0 0 ${x} ${x}`},a(),t(`g`,null,t(`path`,{class:`${g}-progress-graph-circle-rail`,d:_,"stroke-width":s,"stroke-linecap":`round`,fill:`none`,style:v})),t(`g`,null,t(`path`,{class:[`${g}-progress-graph-circle-fill`,u===0&&`${g}-progress-graph-circle-fill--empty`],d:y,"stroke-width":s,"stroke-linecap":`round`,fill:`none`,style:b}))))),d?t(`div`,null,n.default?t(`div`,{class:`${g}-progress-custom-content`,role:`none`},n.default()):l==="default"?t(`div`,{class:`${g}-progress-text`,style:{color:f},role:`none`},t(`span`,{class:`${g}-progress-text__percentage`},u),t(`span`,{class:`${g}-progress-text__unit`},p)):t(`div`,{class:`${g}-progress-icon`,"aria-hidden":!0},t(h,{clsPrefix:g},{default:()=>J[l]}))):null)}}}),X={success:t(F,null),error:t(I,null),warning:t(B,null),info:t(L,null)},Z=i({name:`ProgressLine`,props:{clsPrefix:{type:String,required:!0},percentage:{type:Number,default:0},railColor:String,railStyle:[String,Object],fillColor:[String,Object],status:{type:String,required:!0},indicatorPlacement:{type:String,required:!0},indicatorTextColor:String,unit:{type:String,default:`%`},processing:{type:Boolean,required:!0},showIndicator:{type:Boolean,required:!0},height:[String,Number],railBorderRadius:[String,Number],fillBorderRadius:[String,Number]},setup(e,{slots:n}){let r=l(()=>E(e.height)),i=l(()=>typeof e.fillColor==`object`?`linear-gradient(to right, ${e.fillColor?.stops[0]} , ${e.fillColor?.stops[1]})`:e.fillColor),a=l(()=>e.railBorderRadius===void 0?e.height===void 0?``:E(e.height,{c:.5}):E(e.railBorderRadius)),o=l(()=>e.fillBorderRadius===void 0?e.railBorderRadius===void 0?e.height===void 0?``:E(e.height,{c:.5}):E(e.railBorderRadius):E(e.fillBorderRadius));return()=>{let{indicatorPlacement:s,railColor:c,railStyle:l,percentage:u,unit:d,indicatorTextColor:f,status:p,showIndicator:m,processing:g,clsPrefix:_}=e;return t(`div`,{class:`${_}-progress-content`,role:`none`},t(`div`,{class:`${_}-progress-graph`,"aria-hidden":!0},t(`div`,{class:[`${_}-progress-graph-line`,{[`${_}-progress-graph-line--indicator-${s}`]:!0}]},t(`div`,{class:`${_}-progress-graph-line-rail`,style:[{backgroundColor:c,height:r.value,borderRadius:a.value},l]},t(`div`,{class:[`${_}-progress-graph-line-fill`,g&&`${_}-progress-graph-line-fill--processing`],style:{maxWidth:`${e.percentage}%`,background:i.value,height:r.value,lineHeight:r.value,borderRadius:o.value}},s===`inside`?t(`div`,{class:`${_}-progress-graph-line-indicator`,style:{color:f}},n.default?n.default():`${u}${d}`):null)))),m&&s===`outside`?t(`div`,null,n.default?t(`div`,{class:`${_}-progress-custom-content`,style:{color:f},role:`none`},n.default()):p==="default"?t(`div`,{role:`none`,class:`${_}-progress-icon ${_}-progress-icon--as-text`,style:{color:f}},u,d):t(`div`,{class:`${_}-progress-icon`,"aria-hidden":!0},t(h,{clsPrefix:_},{default:()=>X[p]}))):null)}}});function Q(e,t,n=100){return`m ${n/2} ${n/2-e} a ${e} ${e} 0 1 1 0 ${2*e} a ${e} ${e} 0 1 1 0 -${2*e}`}var $=i({name:`ProgressMultipleCircle`,props:{clsPrefix:{type:String,required:!0},viewBoxWidth:{type:Number,required:!0},percentage:{type:Array,default:[0]},strokeWidth:{type:Number,required:!0},circleGap:{type:Number,required:!0},showIndicator:{type:Boolean,required:!0},fillColor:{type:Array,default:()=>[]},railColor:{type:Array,default:()=>[]},railStyle:{type:Array,default:()=>[]}},setup(e,{slots:n}){let r=l(()=>e.percentage.map((t,n)=>`${Math.PI*t/100*(e.viewBoxWidth/2-e.strokeWidth/2*(1+2*n)-e.circleGap*n)*2}, ${e.viewBoxWidth*8}`)),i=(n,r)=>{let i=e.fillColor[r],a=typeof i==`object`?i.stops[0]:``,o=typeof i==`object`?i.stops[1]:``;return typeof e.fillColor[r]==`object`&&t(`linearGradient`,{id:`gradient-${r}`,x1:`100%`,y1:`0%`,x2:`0%`,y2:`100%`},t(`stop`,{offset:`0%`,"stop-color":a}),t(`stop`,{offset:`100%`,"stop-color":o}))};return()=>{let{viewBoxWidth:a,strokeWidth:o,circleGap:s,showIndicator:c,fillColor:l,railColor:u,railStyle:d,percentage:f,clsPrefix:p}=e;return t(`div`,{class:`${p}-progress-content`,role:`none`},t(`div`,{class:`${p}-progress-graph`,"aria-hidden":!0},t(`div`,{class:`${p}-progress-graph-circle`},t(`svg`,{viewBox:`0 0 ${a} ${a}`},t(`defs`,null,f.map((e,t)=>i(e,t))),f.map((e,n)=>t(`g`,{key:n},t(`path`,{class:`${p}-progress-graph-circle-rail`,d:Q(a/2-o/2*(1+2*n)-s*n,o,a),"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:[{strokeDashoffset:0,stroke:u[n]},d[n]]}),t(`path`,{class:[`${p}-progress-graph-circle-fill`,e===0&&`${p}-progress-graph-circle-fill--empty`],d:Q(a/2-o/2*(1+2*n)-s*n,o,a),"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:{strokeDasharray:r.value[n],strokeDashoffset:0,stroke:typeof l[n]==`object`?`url(#gradient-${n})`:l[n]}})))))),c&&n.default?t(`div`,null,t(`div`,{class:`${p}-progress-text`},n.default())):null)}}}),ee=_([v(`progress`,{display:`inline-block`},[v(`progress-icon`,`
 color: var(--n-icon-color);
 transition: color .3s var(--n-bezier);
 `),g(`line`,`
 width: 100%;
 display: block;
 `,[v(`progress-content`,`
 display: flex;
 align-items: center;
 `,[v(`progress-graph`,{flex:1})]),v(`progress-custom-content`,{marginLeft:`14px`}),v(`progress-icon`,`
 width: 30px;
 padding-left: 14px;
 height: var(--n-icon-size-line);
 line-height: var(--n-icon-size-line);
 font-size: var(--n-icon-size-line);
 `,[g(`as-text`,`
 color: var(--n-text-color-line-outer);
 text-align: center;
 width: 40px;
 font-size: var(--n-font-size);
 padding-left: 4px;
 transition: color .3s var(--n-bezier);
 `)])]),g(`circle, dashboard`,{width:`120px`},[v(`progress-custom-content`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 justify-content: center;
 `),v(`progress-text`,`
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
 `),v(`progress-icon`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 color: var(--n-icon-color);
 font-size: var(--n-icon-size-circle);
 `)]),g(`multiple-circle`,`
 width: 200px;
 color: inherit;
 `,[v(`progress-text`,`
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
 `)]),v(`progress-content`,{position:`relative`}),v(`progress-graph`,{position:`relative`},[v(`progress-graph-circle`,[_(`svg`,{verticalAlign:`bottom`}),v(`progress-graph-circle-fill`,`
 stroke: var(--n-fill-color);
 transition:
 opacity .3s var(--n-bezier),
 stroke .3s var(--n-bezier),
 stroke-dasharray .3s var(--n-bezier);
 `,[g(`empty`,{opacity:0})]),v(`progress-graph-circle-rail`,`
 transition: stroke .3s var(--n-bezier);
 overflow: hidden;
 stroke: var(--n-rail-color);
 `)]),v(`progress-graph-line`,[g(`indicator-inside`,[v(`progress-graph-line-rail`,`
 height: 16px;
 line-height: 16px;
 border-radius: 10px;
 `,[v(`progress-graph-line-fill`,`
 height: inherit;
 border-radius: 10px;
 `),v(`progress-graph-line-indicator`,`
 background: #0000;
 white-space: nowrap;
 text-align: right;
 margin-left: 14px;
 margin-right: 14px;
 height: inherit;
 font-size: 12px;
 color: var(--n-text-color-line-inner);
 transition: color .3s var(--n-bezier);
 `)])]),g(`indicator-inside-label`,`
 height: 16px;
 display: flex;
 align-items: center;
 `,[v(`progress-graph-line-rail`,`
 flex: 1;
 transition: background-color .3s var(--n-bezier);
 `),v(`progress-graph-line-indicator`,`
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
 `)]),v(`progress-graph-line-rail`,`
 position: relative;
 overflow: hidden;
 height: var(--n-rail-height);
 border-radius: 5px;
 background-color: var(--n-rail-color);
 transition: background-color .3s var(--n-bezier);
 `,[v(`progress-graph-line-fill`,`
 background: var(--n-fill-color);
 position: relative;
 border-radius: 5px;
 height: inherit;
 width: 100%;
 max-width: 0%;
 transition:
 background-color .3s var(--n-bezier),
 max-width .2s var(--n-bezier);
 `,[g(`processing`,[_(`&::after`,`
 content: "";
 background-image: var(--n-line-bg-processing);
 animation: progress-processing-animation 2s var(--n-bezier) infinite;
 `)])])])])])]),_(`@keyframes progress-processing-animation`,`
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
 `)]),te=i({name:`Progress`,props:Object.assign(Object.assign({},b.props),{processing:Boolean,type:{type:String,default:`line`},gapDegree:Number,gapOffsetDegree:Number,status:{type:String,default:`default`},railColor:[String,Array],railStyle:[String,Array],color:[String,Array,Object],viewBoxWidth:{type:Number,default:100},strokeWidth:{type:Number,default:7},percentage:[Number,Array],unit:{type:String,default:`%`},showIndicator:{type:Boolean,default:!0},indicatorPosition:{type:String,default:`outside`},indicatorPlacement:{type:String,default:`outside`},indicatorTextColor:String,circleGap:{type:Number,default:1},height:Number,borderRadius:[String,Number],fillBorderRadius:[String,Number],offsetDegree:Number}),setup(e){let t=l(()=>e.indicatorPlacement||e.indicatorPosition),n=l(()=>{if(e.gapDegree||e.gapDegree===0)return e.gapDegree;if(e.type===`dashboard`)return 75}),{mergedClsPrefixRef:r,inlineThemeDisabled:i}=y(e),a=b(`Progress`,`-progress`,ee,z,e,r),o=l(()=>{let{status:t}=e,{common:{cubicBezierEaseInOut:n},self:{fontSize:r,fontSizeCircle:i,railColor:o,railHeight:s,iconSizeCircle:c,iconSizeLine:l,textColorCircle:u,textColorLineInner:d,textColorLineOuter:f,lineBgProcessing:p,fontWeightCircle:m,[C(`iconColor`,t)]:h,[C(`fillColor`,t)]:g}}=a.value;return{"--n-bezier":n,"--n-fill-color":g,"--n-font-size":r,"--n-font-size-circle":i,"--n-font-weight-circle":m,"--n-icon-color":h,"--n-icon-size-circle":c,"--n-icon-size-line":l,"--n-line-bg-processing":p,"--n-rail-color":o,"--n-rail-height":s,"--n-text-color-circle":u,"--n-text-color-line-inner":d,"--n-text-color-line-outer":f}}),s=i?S(`progress`,l(()=>e.status[0]),o,e):void 0;return{mergedClsPrefix:r,mergedIndicatorPlacement:t,gapDeg:n,cssVars:i?void 0:o,themeClass:s?.themeClass,onRender:s?.onRender}},render(){let{type:e,cssVars:n,indicatorTextColor:r,showIndicator:i,status:a,railColor:o,railStyle:s,color:c,percentage:l,viewBoxWidth:u,strokeWidth:d,mergedIndicatorPlacement:f,unit:p,borderRadius:m,fillBorderRadius:h,height:g,processing:_,circleGap:v,mergedClsPrefix:y,gapDeg:b,gapOffsetDegree:x,themeClass:S,$slots:C,onRender:w}=this;return w?.(),t(`div`,{class:[S,`${y}-progress`,`${y}-progress--${e}`,`${y}-progress--${a}`],style:n,"aria-valuemax":100,"aria-valuemin":0,"aria-valuenow":l,role:e===`circle`||e===`line`||e===`dashboard`?`progressbar`:`none`},e===`circle`||e===`dashboard`?t(Y,{clsPrefix:y,status:a,showIndicator:i,indicatorTextColor:r,railColor:o,fillColor:c,railStyle:s,offsetDegree:this.offsetDegree,percentage:l,viewBoxWidth:u,strokeWidth:d,gapDegree:b===void 0?e===`dashboard`?75:0:b,gapOffsetDegree:x,unit:p},C):e===`line`?t(Z,{clsPrefix:y,status:a,showIndicator:i,indicatorTextColor:r,railColor:o,fillColor:c,railStyle:s,percentage:l,processing:_,indicatorPlacement:f,unit:p,fillBorderRadius:h,railBorderRadius:m,height:g},C):e===`multiple-circle`?t($,{clsPrefix:y,strokeWidth:d,railColor:o,fillColor:c,railStyle:s,viewBoxWidth:u,percentage:l,showIndicator:i,circleGap:v},C):null)}}),ne=i({__name:`ScopesView`,setup(i){let{t:l}=T(),h=M(),g=U(),_=d(!1),v=d(!1),y=d([]),b=d(!1),S=d(!1),C=d(``),w=d(null),E=s({page:1,pageSize:20,itemCount:0,showSizePicker:!0,pageSizes:[10,20,50]}),F=s({name:``,subnet:``,start_ip:``,end_ip:``,lease_time:86400,enabled:!0,description:``}),I=[{title:l(`common.name`),key:`name`},{title:l(`dhcp.scopes.subnet`),key:`subnet`},{title:l(`dhcp.scopes.startIp`),key:`start_ip`,width:130},{title:l(`dhcp.scopes.endIp`),key:`end_ip`,width:130},{title:l(`dhcp.scopes.leaseTime`),key:`lease_time`,width:100},{title:l(`dhcp.scopes.activeLeases`),key:`active_leases`,width:100},{title:l(`dhcp.scopes.usage`),key:`usage`,width:120,render:e=>{let n=e.total_addresses>0?Math.round(e.active_leases/e.total_addresses*100):0;return t(te,{type:`line`,percentage:n,indicatorPlacement:`inside`,status:n>90?`error`:n>70?`warning`:`success`})}},{title:l(`common.enabled`),key:`enabled`,width:80,render:e=>t(P,{value:e.enabled,disabled:!g.canWrite(`dhcp`),onUpdateValue:()=>Y(e)})},{title:l(`common.actions`),key:`actions`,width:160,render:e=>t(N,null,{default:()=>[t(x,{size:`small`,onClick:()=>{w.value=e,Object.assign(F,e),b.value=!0}},{default:()=>l(`common.edit`)}),t(x,{size:`small`,type:`error`,disabled:!g.canDelete(`dhcp`),onClick:()=>{C.value=e.id,S.value=!0}},{default:()=>l(`common.delete`)})]})}];async function L(){_.value=!0;try{let e=await W({page:E.page,page_size:E.pageSize});y.value=e.data,E.itemCount=e.meta.total}catch(e){h.error(e instanceof Error?e.message:l(`common.failed`))}finally{_.value=!1}}function z(e){E.page=e,L()}function B(e){E.pageSize=e,E.page=1,L()}function J(){w.value=null,Object.assign(F,{name:``,subnet:``,start_ip:``,end_ip:``,lease_time:86400,enabled:!0,description:``}),b.value=!0}async function Y(e){try{await G(e.id,{enabled:!e.enabled}),h.success(l(`common.updateSuccess`)),L()}catch(e){h.error(e instanceof Error?e.message:l(`common.failed`))}}async function X(){v.value=!0;try{w.value?(await G(w.value.id,F),h.success(l(`common.updateSuccess`))):(await K(F),h.success(l(`common.createSuccess`))),b.value=!1,w.value=null,L()}catch(e){h.error(e instanceof Error?e.message:l(`common.failed`))}finally{v.value=!1}}async function Z(){try{await q(C.value),h.success(l(`common.deleteSuccess`)),L()}catch(e){h.error(e instanceof Error?e.message:l(`common.failed`))}S.value=!1}return r(L),(t,r)=>{let i=k,s=A,d=O,h=j,C=D,T=R;return a(),c(`div`,null,[p(V,{title:u(l)(`dhcp.scopes.title`)},{default:o(()=>[u(g).canWrite(`dhcp`)?(a(),f(u(x),{key:0,type:`primary`,onClick:J},{default:o(()=>[e(n(u(l)(`dhcp.scopes.createScope`)),1)]),_:1})):m(``,!0)]),_:1},8,[`title`]),p(i,{columns:I,data:y.value,loading:_.value,pagination:E,"row-key":e=>e.id,"onUpdate:page":z,"onUpdate:pageSize":B},null,8,[`data`,`loading`,`pagination`,`row-key`]),b.value?(a(),f(T,{key:0,show:b.value,"onUpdate:show":r[8]||=e=>b.value=e,preset:`card`,title:w.value?u(l)(`dhcp.scopes.editScope`):u(l)(`dhcp.scopes.createScope`),style:{width:`550px`}},{footer:o(()=>[p(u(N),{justify:`end`},{default:o(()=>[p(u(x),{onClick:r[7]||=e=>b.value=!1},{default:o(()=>[e(n(u(l)(`common.cancel`)),1)]),_:1}),p(u(x),{type:`primary`,loading:v.value,onClick:X},{default:o(()=>[e(n(u(l)(`common.save`)),1)]),_:1},8,[`loading`])]),_:1})]),default:o(()=>[p(C,{model:F,"label-placement":`left`,"label-width":`100px`},{default:o(()=>[p(d,{label:u(l)(`common.name`)},{default:o(()=>[p(s,{value:F.name,"onUpdate:value":r[0]||=e=>F.name=e},null,8,[`value`])]),_:1},8,[`label`]),p(d,{label:u(l)(`dhcp.scopes.subnet`)},{default:o(()=>[p(s,{value:F.subnet,"onUpdate:value":r[1]||=e=>F.subnet=e,placeholder:`192.168.1.0/24`},null,8,[`value`])]),_:1},8,[`label`]),p(d,{label:u(l)(`dhcp.scopes.startIp`)},{default:o(()=>[p(s,{value:F.start_ip,"onUpdate:value":r[2]||=e=>F.start_ip=e},null,8,[`value`])]),_:1},8,[`label`]),p(d,{label:u(l)(`dhcp.scopes.endIp`)},{default:o(()=>[p(s,{value:F.end_ip,"onUpdate:value":r[3]||=e=>F.end_ip=e},null,8,[`value`])]),_:1},8,[`label`]),p(d,{label:u(l)(`dhcp.scopes.leaseTime`)},{default:o(()=>[p(h,{value:F.lease_time,"onUpdate:value":r[4]||=e=>F.lease_time=e,min:60,style:{width:`100%`}},null,8,[`value`])]),_:1},8,[`label`]),p(d,{label:u(l)(`common.description`)},{default:o(()=>[p(s,{value:F.description,"onUpdate:value":r[5]||=e=>F.description=e,type:`textarea`},null,8,[`value`])]),_:1},8,[`label`]),p(d,{label:u(l)(`common.enabled`)},{default:o(()=>[p(u(P),{value:F.enabled,"onUpdate:value":r[6]||=e=>F.enabled=e},null,8,[`value`])]),_:1},8,[`label`])]),_:1},8,[`model`])]),_:1},8,[`show`,`title`])):m(``,!0),p(H,{show:S.value,message:u(l)(`common.deleteConfirm`),onConfirm:Z,onCancel:r[9]||=e=>S.value=!1},null,8,[`show`,`message`])])}}});export{ne as default};