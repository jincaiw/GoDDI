import{E as e,g as t,k as n}from"./vue.runtime.esm-bundler-B46jYzg4.js";import{Dn as r,Nr as i,On as a,ir as o,jr as s,kr as c,un as l}from"./router-BRUVYu7g.js";import{t as u}from"./index-DhifKGEo.js";var d=c(`text`,`
 transition: color .3s var(--n-bezier);
 color: var(--n-text-color);
`,[s(`strong`,`
 font-weight: var(--n-font-weight-strong);
 `),s(`italic`,{fontStyle:`italic`}),s(`underline`,{textDecoration:`underline`}),s(`code`,`
 line-height: 1.4;
 display: inline-block;
 font-family: var(--n-font-famliy-mono);
 transition: 
 color .3s var(--n-bezier),
 border-color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 box-sizing: border-box;
 padding: .05em .35em 0 .35em;
 border-radius: var(--n-code-border-radius);
 font-size: .9em;
 color: var(--n-code-text-color);
 background-color: var(--n-code-color);
 border: var(--n-code-border);
 `)]),f=e({name:`Text`,props:Object.assign(Object.assign({},l.props),{code:Boolean,type:{type:String,default:`default`},delete:Boolean,strong:Boolean,italic:Boolean,underline:Boolean,depth:[String,Number],tag:String,as:{type:String,validator:()=>!0,default:void 0}}),setup(e){let{mergedClsPrefixRef:n,inlineThemeDisabled:s}=a(e),c=l(`Typography`,`-text`,d,u,e,n),f=t(()=>{let{depth:t,type:n}=e,r=n===`default`?t===void 0?`textColor`:`textColor${t}Depth`:i(`textColor`,n),{common:{fontWeightStrong:a,fontFamilyMono:o,cubicBezierEaseInOut:s},self:{codeTextColor:l,codeBorderRadius:u,codeColor:d,codeBorder:f,[r]:p}}=c.value;return{"--n-bezier":s,"--n-text-color":p,"--n-font-weight-strong":a,"--n-font-famliy-mono":o,"--n-code-border-radius":u,"--n-code-text-color":l,"--n-code-color":d,"--n-code-border":f}}),p=s?r(`text`,t(()=>`${e.type[0]}${e.depth||``}`),f,e):void 0;return{mergedClsPrefix:n,compitableTag:o(e,[`as`,`tag`]),cssVars:s?void 0:f,themeClass:p?.themeClass,onRender:p?.onRender}},render(){var e,t;let{mergedClsPrefix:r}=this;(e=this.onRender)==null||e.call(this);let i=[`${r}-text`,this.themeClass,{[`${r}-text--code`]:this.code,[`${r}-text--delete`]:this.delete,[`${r}-text--strong`]:this.strong,[`${r}-text--italic`]:this.italic,[`${r}-text--underline`]:this.underline}],a=(t=this.$slots).default?.call(t);return this.code?n(`code`,{class:i,style:this.cssVars},this.delete?n(`del`,null,a):a):this.delete?n(`del`,{class:i,style:this.cssVars},a):n(this.compitableTag||`span`,{class:i,style:this.cssVars},a)}});export{f as t};